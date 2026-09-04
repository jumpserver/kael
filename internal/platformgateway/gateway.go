package platformgateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jumpserver/kael/internal/domain"
	"github.com/jumpserver/kael/internal/ports"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	searchTool = "search_core_api"
	callTool   = "call_core_api"
)

type Config struct {
	CoreURL         string
	CoreTLSVerify   bool
	DelegationKey   string
	DelegationKeyID string
	Issuer          string
	Audience        string
	CACert          string
	ClientCert      string
	ClientKey       string
	AllowedMethods  map[string]bool
	RegistryTTL     time.Duration
	Timeout         time.Duration
	MaxResponse     int64
}

type parameter struct {
	Name     string         `json:"name"`
	Required bool           `json:"required"`
	Style    string         `json:"style,omitempty"`
	Explode  bool           `json:"explode"`
	Schema   map[string]any `json:"schema,omitempty"`
}

type operation struct {
	ID           string
	Method       string
	Path         string
	Summary      string
	Description  string
	Tags         []string
	PathParams   []parameter
	QueryParams  []parameter
	Body         map[string]any
	BodyRequired bool
}

type registry struct {
	Hash       string
	Operations map[string]operation
	LoadedAt   time.Time
}

type Gateway struct {
	config   Config
	client   *http.Client
	mu       sync.RWMutex
	current  *registry
	versions map[string]*registry
}

func New(config Config) (*Gateway, error) {
	parsed, err := url.Parse(config.CoreURL)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("platform gateway Core URL is invalid")
	}
	if len(config.DelegationKey) < 32 || config.DelegationKeyID == "" {
		return nil, fmt.Errorf("platform gateway delegation key is invalid")
	}
	if config.Issuer == "" {
		config.Issuer = "jumpserver-ai"
	}
	if config.Audience == "" {
		config.Audience = "jumpserver-core"
	}
	if config.RegistryTTL <= 0 {
		config.RegistryTTL = time.Hour
	}
	if config.Timeout <= 0 {
		config.Timeout = 15 * time.Second
	}
	if config.MaxResponse <= 0 {
		config.MaxResponse = 1024 * 1024
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: !config.CoreTLSVerify} //nolint:gosec -- deployment-controlled compatibility option
	if config.CACert != "" {
		content, readErr := os.ReadFile(config.CACert)
		if readErr != nil {
			return nil, fmt.Errorf("read platform gateway CA certificate: %w", readErr)
		}
		roots, loadErr := x509.SystemCertPool()
		if loadErr != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(content) {
			return nil, fmt.Errorf("platform gateway CA certificate is invalid")
		}
		tlsConfig.RootCAs = roots
	}
	if config.ClientCert != "" || config.ClientKey != "" {
		if config.ClientCert == "" || config.ClientKey == "" {
			return nil, fmt.Errorf("platform gateway client certificate configuration is incomplete")
		}
		certificate, loadErr := tls.LoadX509KeyPair(config.ClientCert, config.ClientKey)
		if loadErr != nil {
			return nil, fmt.Errorf("load platform gateway client certificate: %w", loadErr)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = tlsConfig
	client := &http.Client{Transport: transport, Timeout: config.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Gateway{config: config, client: client, versions: make(map[string]*registry)}, nil
}

func (g *Gateway) Registrations(ctx context.Context, principal domain.Principal, profile string) ([]domain.Registration, error) {
	if !profileEnabled(profile, principal) {
		return nil, fmt.Errorf("platform profile is not available")
	}
	registry, err := g.load(ctx, false)
	if err != nil {
		return nil, err
	}
	return registrationsFor(registry, profile), nil
}

func registrationsFor(registry *registry, profile string) []domain.Registration {
	searchSchema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","minLength":1,"maxLength":512},"progress":{"type":"string","maxLength":512},"action":{"type":"string","maxLength":128}},"required":["query","progress","action"],"additionalProperties":false}`)
	callSchema := json.RawMessage(`{"type":"object","properties":{"operation_id":{"type":"string","minLength":1,"maxLength":256},"progress":{"type":"string","maxLength":512},"action":{"type":"string","maxLength":128},"path_params":{"type":"object"},"query_params":{"type":"object"},"body":{"oneOf":[{"type":"object"},{"type":"array"}]}},"required":["operation_id","progress","action"],"additionalProperties":false}`)
	readAnnotations, _ := json.Marshal(domain.ToolAnnotations{ReadOnly: true, Idempotent: true})
	writeAnnotations, _ := json.Marshal(domain.ToolAnnotations{OpenWorld: true})
	definitions := []domain.Registration{
		{ID: "platform-gateway:search:" + registry.Hash, BindingKind: "service", ExecutionBindingID: "platform-gateway:" + registry.Hash, ClientKey: searchTool, Name: searchTool, Description: "Search server-authorized JumpServer Core operations by user intent.", InputSchema: searchSchema, DefinitionVersion: registry.Hash, Namespace: profile, Risk: "read", AnnotationsJSON: readAnnotations, State: "active"},
		{ID: "platform-gateway:call:" + registry.Hash, BindingKind: "service", ExecutionBindingID: "platform-gateway:" + registry.Hash, ClientKey: callTool, Name: callTool, Description: "Call one server-authorized Core operation selected from search results.", InputSchema: callSchema, DefinitionVersion: registry.Hash, Namespace: profile, Risk: "dangerous", RequiresConfirmation: true, AnnotationsJSON: writeAnnotations, State: "active"},
	}
	for index := range definitions {
		definitions[index].DefinitionDigest, _ = domain.HashValue(map[string]any{"name": definitions[index].Name, "description": definitions[index].Description, "schema": definitions[index].InputSchema, "version": registry.Hash})
	}
	return definitions
}

func (g *Gateway) Prepare(ctx context.Context, request ports.CapabilityRequest) (ports.CapabilityPolicy, error) {
	registry, operation, arguments, err := g.resolveRequest(ctx, request)
	if err != nil {
		return ports.CapabilityPolicy{}, err
	}
	if request.Registration.Name == searchTool {
		return ports.CapabilityPolicy{Risk: "read"}, nil
	}
	if operation == nil {
		return ports.CapabilityPolicy{}, fmt.Errorf("operation is unavailable")
	}
	risk := methodRisk(operation.Method)
	preview, _ := json.Marshal(map[string]any{"operation_id": operation.ID, "method": operation.Method, "path": operation.Path, "summary": operation.Summary, "arguments": sanitize(arguments, 0)})
	_ = registry
	return ports.CapabilityPolicy{Risk: risk, RequiresConfirmation: risk != "read", Preview: preview}, nil
}

func (g *Gateway) Execute(ctx context.Context, request ports.CapabilityRequest) (ports.CapabilityResult, error) {
	registry, operation, arguments, err := g.resolveRequest(ctx, request)
	if err != nil {
		return ports.CapabilityResult{}, err
	}
	if request.Registration.Name == searchTool {
		query, _ := arguments["query"].(string)
		candidates := searchOperations(registry, request.Profile, query, 5, g.config.AllowedMethods)
		encoded, _ := json.Marshal(map[string]any{"operations": candidates, "registry_version": registry.Hash})
		return ports.CapabilityResult{Status: "success", Result: encoded, ExecutorAuditReference: "platform-search:" + uuid.NewString()}, nil
	}
	path, query, body, err := buildRequest(*operation, arguments)
	if err != nil {
		return ports.CapabilityResult{}, err
	}
	var bodyReader io.Reader
	var bodyBytes []byte
	if operation.Method != http.MethodGet && len(operation.Body) > 0 {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return ports.CapabilityResult{}, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}
	endpoint := strings.TrimRight(g.config.CoreURL, "/") + path
	if query != "" {
		endpoint += "?" + query
	}
	httpRequest, err := http.NewRequestWithContext(ctx, operation.Method, endpoint, bodyReader)
	if err != nil {
		return ports.CapabilityResult{}, err
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("X-JMS-ORG", request.Principal.OrganizationID)
	httpRequest.Header.Set("X-JMS-AI-Operation", operation.ID)
	if len(bodyBytes) > 0 {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	requestHash := bindingHash(operation.Method, path, []byte(query), bodyBytes)
	httpRequest.Header.Set("X-JMS-AI-Delegation", g.delegation(request, *operation, path, requestHash))
	response, err := g.client.Do(httpRequest)
	if err != nil {
		return ports.CapabilityResult{}, err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, g.config.MaxResponse+1))
	if err != nil {
		return ports.CapabilityResult{}, err
	}
	if int64(len(content)) > g.config.MaxResponse {
		return ports.CapabilityResult{}, fmt.Errorf("Core response exceeds the configured limit")
	}
	var value any
	if len(content) > 0 && json.Unmarshal(content, &value) != nil {
		value = boundedString(string(content), 8192)
	}
	clean := sanitize(value, 0)
	result := map[string]any{"ok": response.StatusCode >= 200 && response.StatusCode < 300, "status_code": response.StatusCode, "operation_id": operation.ID, "data": clean}
	encoded, _ := json.Marshal(result)
	if len(encoded) > domain.MaxToolResultBytes {
		encoded, _ = json.Marshal(map[string]any{"ok": result["ok"], "status_code": response.StatusCode, "operation_id": operation.ID, "data": map[string]any{"truncated": true, "preview": boundedString(string(encoded), 32*1024)}})
	}
	cards, _ := json.Marshal([]any{buildResultCard(*operation, response.StatusCode, clean)})
	if len(cards) > 64*1024 {
		cards, _ = json.Marshal([]any{map[string]any{"type": "detail", "title": operation.Summary, "source": map[string]any{"type": "core_api", "operation_id": operation.ID, "method": operation.Method, "path": operation.Path, "status_code": response.StatusCode}, "content": map[string]any{"truncated": true, "preview": boundedString(string(cards), 32*1024)}}})
	}
	return ports.CapabilityResult{Status: "success", Result: encoded, ResultCards: cards, ExecutorAuditReference: "core-api:" + uuid.NewString()}, nil
}

func buildResultCard(operation operation, status int, data any) map[string]any {
	rows := resultRows(data)
	kind := "detail"
	content := data
	if len(rows) > 0 {
		kind = "table"
		for _, marker := range []string{"session", "command", "audit", "login_log", "operate_log", "job_log"} {
			if strings.Contains(operation.ID, marker) {
				kind = "timeline"
				break
			}
		}
		preferred := []string(nil)
		if operation.ID == "assets_assets_list" || operation.ID == "assets_hosts_list" || operation.ID == "assets_nodes_assets_list" {
			preferred = []string{"name", "address", "platform", "accounts_amount", "is_active", "date_verified"}
		}
		content = tableContent(rows, preferred)
		if object, ok := data.(map[string]any); ok {
			if total, ok := object["count"].(json.Number); ok {
				content.(map[string]any)["total"] = total
			} else if total, ok := object["count"].(float64); ok {
				content.(map[string]any)["total"] = total
			}
		}
	} else if strings.Contains(operation.ID, "metric") || strings.Contains(operation.ID, "status") {
		kind = "metric"
	}
	title := operation.Summary
	if title == "" {
		title = operation.ID
	}
	if operation.ID == "assets_assets_list" || operation.ID == "assets_hosts_list" || operation.ID == "assets_nodes_assets_list" {
		title = "Assets"
	}
	return map[string]any{"type": kind, "title": title, "source": map[string]any{"type": "core_api", "operation_id": operation.ID, "method": operation.Method, "path": operation.Path, "status_code": status}, "content": content}
}

func resultRows(data any) []any {
	if object, ok := data.(map[string]any); ok {
		rows, _ := object["results"].([]any)
		return rows
	}
	rows, _ := data.([]any)
	return rows
}

func tableContent(rows []any, preferred []string) map[string]any {
	objects := make([]map[string]any, 0, min(len(rows), 20))
	for _, row := range rows {
		if object, ok := row.(map[string]any); ok {
			objects = append(objects, object)
			if len(objects) == 20 {
				break
			}
		}
	}
	columns := make([]string, 0, 10)
	seen := map[string]bool{}
	if len(preferred) > 0 {
		for _, key := range preferred {
			for _, row := range objects {
				if displayScalar(row[key]) != nil {
					columns = append(columns, key)
					break
				}
			}
		}
	} else {
		for _, row := range objects {
			for key, value := range row {
				if !seen[key] && displayScalar(value) != nil {
					seen[key] = true
					columns = append(columns, key)
					if len(columns) == 10 {
						break
					}
				}
			}
			if len(columns) == 10 {
				break
			}
		}
	}
	tableRows := make([]map[string]any, 0, len(objects))
	for index, row := range objects {
		item := make(map[string]any, len(columns)+1)
		for _, column := range columns {
			item[column] = displayScalar(row[column])
		}
		item["_key"] = firstValue(row["id"], row["pk"], index)
		tableRows = append(tableRows, item)
	}
	result := map[string]any{"columns": columns, "rows": tableRows}
	if len(preferred) > 0 {
		result["variant"] = "assets"
	}
	return result
}

func displayScalar(value any) any {
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"name", "label", "display_name", "value"} {
			if scalar(object[key]) {
				return object[key]
			}
		}
		return nil
	}
	if scalar(value) {
		return value
	}
	return nil
}

func scalar(value any) bool {
	switch value.(type) {
	case nil, string, bool, float64, json.Number:
		return true
	default:
		return false
	}
}

func firstValue(values ...any) any {
	for _, value := range values {
		if value != nil && value != "" {
			return value
		}
	}
	return nil
}

func (g *Gateway) Refresh(ctx context.Context) (map[string]any, error) {
	registry, err := g.load(ctx, true)
	if err != nil {
		return nil, err
	}
	return map[string]any{"registry_version": registry.Hash, "operations": len(registry.Operations), "loaded_at": registry.LoadedAt}, nil
}

func (g *Gateway) resolveRequest(ctx context.Context, request ports.CapabilityRequest) (*registry, *operation, map[string]any, error) {
	if !profileEnabled(request.Profile, request.Principal) {
		return nil, nil, nil, fmt.Errorf("platform profile is not available")
	}
	var arguments map[string]any
	decoder := json.NewDecoder(bytes.NewReader(request.Arguments))
	decoder.UseNumber()
	if decoder.Decode(&arguments) != nil || arguments == nil || containsSensitive(arguments) {
		return nil, nil, nil, fmt.Errorf("capability arguments are invalid")
	}
	version := strings.TrimPrefix(request.Registration.ExecutionBindingID, "platform-gateway:")
	registry, err := g.version(ctx, version)
	if err != nil {
		return nil, nil, nil, err
	}
	registrations := registrationsFor(registry, request.Profile)
	validDefinition := false
	for _, registration := range registrations {
		if registration.Name == request.Registration.Name &&
			registration.DefinitionDigest == request.Registration.DefinitionDigest &&
			registration.DefinitionVersion == request.Registration.DefinitionVersion &&
			registration.ExecutionBindingID == request.Registration.ExecutionBindingID {
			validDefinition = true
			break
		}
	}
	if !validDefinition {
		return nil, nil, nil, fmt.Errorf("capability definition is stale")
	}
	if request.Registration.Name == searchTool {
		return registry, nil, arguments, nil
	}
	if request.Registration.Name != callTool {
		return nil, nil, nil, fmt.Errorf("unknown service capability")
	}
	operationID, _ := arguments["operation_id"].(string)
	operation, ok := registry.Operations[operationID]
	if !ok || !operationAllowed(request.Profile, operation, g.config.AllowedMethods) {
		return nil, nil, nil, fmt.Errorf("Core operation is not allowed")
	}
	return registry, &operation, arguments, nil
}

func (g *Gateway) version(ctx context.Context, hash string) (*registry, error) {
	g.mu.RLock()
	value := g.versions[hash]
	g.mu.RUnlock()
	if value != nil {
		return value, nil
	}
	loaded, err := g.load(ctx, false)
	if err != nil {
		return nil, err
	}
	if loaded.Hash != hash {
		return nil, fmt.Errorf("platform registry version is unavailable")
	}
	return loaded, nil
}

func (g *Gateway) load(ctx context.Context, force bool) (*registry, error) {
	g.mu.RLock()
	current := g.current
	g.mu.RUnlock()
	if current != nil && !force && time.Since(current.LoadedAt) < g.config.RegistryTTL {
		return current, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(g.config.CoreURL, "/")+"/api/swagger.json", nil)
	if err != nil {
		return nil, err
	}
	response, err := g.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Core OpenAPI returned HTTP %d", response.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, 32*1024*1024+1))
	if err != nil || len(content) > 32*1024*1024 {
		return nil, fmt.Errorf("Core OpenAPI response is invalid")
	}
	var schema map[string]any
	if json.Unmarshal(content, &schema) != nil {
		return nil, fmt.Errorf("Core OpenAPI response is invalid")
	}
	operations, err := parseOperations(schema)
	if err != nil {
		return nil, err
	}
	hash := domain.HashBytes(content)
	loaded := &registry{Hash: hash, Operations: operations, LoadedAt: time.Now().UTC()}
	g.mu.Lock()
	g.current, g.versions[hash] = loaded, loaded
	if len(g.versions) > 4 {
		oldestHash, oldestTime := "", loaded.LoadedAt
		for candidate, version := range g.versions {
			if candidate != hash && !version.LoadedAt.After(oldestTime) {
				oldestHash, oldestTime = candidate, version.LoadedAt
			}
		}
		if oldestHash != "" {
			delete(g.versions, oldestHash)
		}
	}
	g.mu.Unlock()
	return loaded, nil
}

func parseOperations(schema map[string]any) (map[string]operation, error) {
	paths, _ := schema["paths"].(map[string]any)
	result := make(map[string]operation)
	for path, rawPath := range paths {
		pathItem, _ := resolve(schema, rawPath, 0).(map[string]any)
		for method, rawOperation := range pathItem {
			upper := strings.ToUpper(method)
			if !map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}[upper] {
				continue
			}
			item, _ := resolve(schema, rawOperation, 0).(map[string]any)
			if item == nil {
				continue
			}
			id, _ := item["operationId"].(string)
			if id == "" {
				id = strings.ToLower(upper) + "_" + regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(strings.Trim(path, "/"), "_")
			}
			entry := operation{ID: id, Method: upper, Path: path, Summary: stringValue(item["summary"]), Description: stringValue(item["description"]), Tags: stringSlice(item["tags"])}
			parameters := append(anySlice(pathItem["parameters"]), anySlice(item["parameters"])...)
			for _, rawParameter := range parameters {
				value, _ := resolve(schema, rawParameter, 0).(map[string]any)
				location := stringValue(value["in"])
				name := stringValue(value["name"])
				if name == "" {
					continue
				}
				parameterSchema, _ := resolve(schema, value["schema"], 0).(map[string]any)
				style := stringValue(value["style"])
				if style == "" {
					style = "form"
				}
				parameter := parameter{Name: name, Required: boolValue(value["required"]), Style: style, Explode: boolDefault(value["explode"], style == "form"), Schema: parameterSchema}
				if location == "path" {
					entry.PathParams = append(entry.PathParams, parameter)
				} else if location == "query" {
					entry.QueryParams = append(entry.QueryParams, parameter)
				}
			}
			requestBody, _ := resolve(schema, item["requestBody"], 0).(map[string]any)
			content, _ := requestBody["content"].(map[string]any)
			media, _ := content["application/json"].(map[string]any)
			entry.Body, _ = resolve(schema, media["schema"], 0).(map[string]any)
			entry.BodyRequired = boolValue(requestBody["required"])
			result[id] = entry
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("Core OpenAPI contains no supported operations")
	}
	return result, nil
}

func resolve(root map[string]any, value any, depth int) any {
	if depth > 12 {
		return map[string]any{}
	}
	switch item := value.(type) {
	case map[string]any:
		if reference, _ := item["$ref"].(string); strings.HasPrefix(reference, "#/") {
			var current any = root
			for _, part := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
				object, _ := current.(map[string]any)
				current = object[strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")]
			}
			return resolve(root, current, depth+1)
		}
		result := make(map[string]any, len(item))
		for key, child := range item {
			result[key] = resolve(root, child, depth+1)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for index, child := range item {
			result[index] = resolve(root, child, depth+1)
		}
		return result
	default:
		return value
	}
}

func buildRequest(operation operation, arguments map[string]any) (string, string, any, error) {
	pathValues, _ := arguments["path_params"].(map[string]any)
	if pathValues == nil {
		pathValues = map[string]any{}
	}
	queryValues, _ := arguments["query_params"].(map[string]any)
	if queryValues == nil {
		queryValues = map[string]any{}
	}
	path := operation.Path
	allowedPath := map[string]parameter{}
	for _, parameter := range operation.PathParams {
		allowedPath[parameter.Name] = parameter
		value, ok := pathValues[parameter.Name]
		if parameter.Required && !ok {
			return "", "", nil, fmt.Errorf("required path parameter is missing")
		}
		if ok {
			if err := validateValue(parameter.Schema, value); err != nil {
				return "", "", nil, err
			}
			path = strings.ReplaceAll(path, "{"+parameter.Name+"}", url.PathEscape(fmt.Sprint(value)))
		}
	}
	for name := range pathValues {
		if _, ok := allowedPath[name]; !ok {
			return "", "", nil, fmt.Errorf("unknown path parameter")
		}
	}
	if strings.ContainsAny(path, "{}") || !strings.HasPrefix(path, "/api/v1/") {
		return "", "", nil, fmt.Errorf("Core path is invalid")
	}
	allowedQuery := map[string]parameter{}
	for _, parameter := range operation.QueryParams {
		allowedQuery[parameter.Name] = parameter
		if parameter.Required {
			if _, ok := queryValues[parameter.Name]; !ok {
				return "", "", nil, fmt.Errorf("required query parameter is missing")
			}
		}
	}
	query := url.Values{}
	for name, value := range queryValues {
		parameter, ok := allowedQuery[name]
		if !ok {
			return "", "", nil, fmt.Errorf("unknown query parameter")
		}
		if err := validateValue(parameter.Schema, value); err != nil {
			return "", "", nil, err
		}
		switch item := value.(type) {
		case []any:
			values := make([]string, 0, len(item))
			for _, child := range item {
				encoded, encodeErr := queryScalar(child)
				if encodeErr != nil {
					return "", "", nil, encodeErr
				}
				values = append(values, encoded)
			}
			switch {
			case parameter.Style == "form" && parameter.Explode:
				for _, encoded := range values {
					query.Add(name, encoded)
				}
			case parameter.Style == "form":
				query.Set(name, strings.Join(values, ","))
			case parameter.Style == "spaceDelimited":
				query.Set(name, strings.Join(values, " "))
			case parameter.Style == "pipeDelimited":
				query.Set(name, strings.Join(values, "|"))
			default:
				return "", "", nil, fmt.Errorf("unsupported query parameter style")
			}
		case map[string]any:
			keys := make([]string, 0, len(item))
			for key := range item {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			flattened := make([]string, 0, len(keys)*2)
			for _, key := range keys {
				encoded, encodeErr := queryScalar(item[key])
				if encodeErr != nil {
					return "", "", nil, encodeErr
				}
				switch {
				case parameter.Style == "deepObject":
					query.Set(name+"["+key+"]", encoded)
				case parameter.Style == "form" && parameter.Explode:
					query.Set(key, encoded)
				case parameter.Style == "form":
					flattened = append(flattened, key, encoded)
				default:
					return "", "", nil, fmt.Errorf("unsupported query parameter style")
				}
			}
			if parameter.Style == "form" && !parameter.Explode {
				query.Set(name, strings.Join(flattened, ","))
			}
		default:
			encoded, encodeErr := queryScalar(item)
			if encodeErr != nil {
				return "", "", nil, encodeErr
			}
			query.Set(name, encoded)
		}
	}
	body, hasBody := arguments["body"]
	if !hasBody {
		body = map[string]any{}
	}
	if operation.BodyRequired && !hasBody {
		return "", "", nil, fmt.Errorf("request body is required")
	}
	if len(operation.Body) > 0 {
		if err := validateValue(operation.Body, body); err != nil {
			return "", "", nil, err
		}
	} else if hasBody && body != nil && !emptyObject(body) {
		return "", "", nil, fmt.Errorf("operation does not accept a body")
	}
	return path, query.Encode(), body, nil
}

func queryScalar(value any) (string, error) {
	switch item := value.(type) {
	case nil:
		return "", nil
	case bool:
		if item {
			return "true", nil
		}
		return "false", nil
	case string, json.Number, float64:
		return fmt.Sprint(item), nil
	default:
		return "", fmt.Errorf("query parameter value must be scalar")
	}
}

func emptyObject(value any) bool {
	object, ok := value.(map[string]any)
	return ok && len(object) == 0
}

func validateValue(schema map[string]any, value any) error {
	if len(schema) == 0 {
		return nil
	}
	raw, _ := json.Marshal(schema)
	compiler := jsonschema.NewCompiler()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if err = compiler.AddResource("schema.json", document); err != nil {
		return err
	}
	compiled, err := compiler.Compile("schema.json")
	if err != nil {
		return err
	}
	return compiled.Validate(value)
}

func (g *Gateway) delegation(request ports.CapabilityRequest, operation operation, path, requestHash string) string {
	now := time.Now().Unix()
	payload := map[string]any{"issuer": g.config.Issuer, "audience": g.config.Audience, "key_id": g.config.DelegationKeyID, "user_id": request.Principal.SubjectID, "org_id": request.Principal.OrganizationID, "conversation_id": request.ConversationID, "approval_id": request.ApprovalID, "allowed_operation_id": operation.ID, "method": operation.Method, "path": path, "request_hash": requestHash, "issued_at": now, "expires_at": now + min64(60, int64(g.config.Timeout.Seconds())+5), "nonce": strings.ReplaceAll(uuid.NewString(), "-", "")}
	encodedPayload, _ := domain.CanonicalJSON(payload)
	encoded := base64.RawURLEncoding.EncodeToString(encodedPayload)
	signer := hmac.New(sha256.New, []byte(g.config.DelegationKey))
	_, _ = signer.Write([]byte(encoded))
	return encoded + "." + hex.EncodeToString(signer.Sum(nil))
}

func bindingHash(method, path string, query, body []byte) string {
	hasher := sha256.New()
	for index, value := range [][]byte{[]byte(strings.ToUpper(method)), []byte(path), query, body} {
		if index > 0 {
			_, _ = hasher.Write([]byte{0})
		}
		_, _ = hasher.Write(value)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}
func methodRisk(method string) string {
	if method == http.MethodGet {
		return "read"
	}
	if method == http.MethodDelete {
		return "dangerous"
	}
	return "write"
}

var scopes = map[string]map[string]bool{
	"platform.asset":         set("assets_assets_list", "assets_assets_retrieve", "assets_categories_list", "assets_hosts_list", "assets_hosts_retrieve", "assets_nodes_assets_list", "assets_nodes_list", "assets_nodes_retrieve", "assets_platforms_list", "assets_platforms_retrieve", "assets_protocols_list"),
	"platform.session_audit": set("audits_activities_list", "audits_login_logs_list", "audits_login_logs_retrieve", "audits_my_login_logs_list", "audits_my_login_logs_retrieve", "audits_operate_logs_list", "audits_operate_logs_retrieve", "audits_service_access_logs_list", "audits_service_access_logs_retrieve", "audits_tickets_list", "audits_tickets_retrieve", "terminal_commands_list", "terminal_commands_retrieve", "terminal_sessions_list", "terminal_sessions_retrieve", "terminal_tasks_list", "terminal_tasks_retrieve"),
	"platform.ops":           set("audits_job_logs_list", "audits_job_logs_retrieve", "audits_jobs_list", "audits_jobs_retrieve", "ops_jobs_list", "ops_jobs_retrieve", "ops_tasks_list", "ops_tasks_retrieve", "terminal_components_metrics_retrieve", "terminal_terminals_list", "terminal_terminals_retrieve"),
}

func set(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
func profileEnabled(profile string, principal domain.Principal) bool {
	return profile == "platform.management" && (principal.IsSuperuser || principal.IsOrgAdmin) || scopes[profile] != nil
}
func operationAllowed(profile string, operation operation, methods map[string]bool) bool {
	if !methods[operation.Method] || sensitivePath(operation.Path) {
		return false
	}
	if profile == "platform.management" {
		return true
	}
	return scopes[profile][operation.ID]
}
func sensitivePath(path string) bool {
	lowered := strings.ToLower(path)
	for _, marker := range []string{"password", "secret", "private-key", "private_key", "access-key", "access_key", "token", "credential", "backup"} {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return strings.Contains(lowered, "chat-ai")
}

func searchOperations(registry *registry, profile, query string, limit int, methods map[string]bool) []map[string]any {
	tokens := tokenize(query)
	type scored struct {
		score     int
		operation operation
	}
	ranked := []scored{}
	for _, operation := range registry.Operations {
		if !operationAllowed(profile, operation, methods) {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{operation.ID, operation.Summary, operation.Description, operation.Path, operation.Method, strings.Join(operation.Tags, " ")}, " "))
		score := 0
		for _, token := range tokens {
			if strings.Contains(haystack, token) {
				score++
			}
		}
		if score > 0 {
			ranked = append(ranked, scored{score: score, operation: operation})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].operation.ID < ranked[j].operation.ID
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	result := make([]map[string]any, 0, len(ranked))
	for _, item := range ranked {
		operation := item.operation
		result = append(result, map[string]any{"operation_id": operation.ID, "method": operation.Method, "path": operation.Path, "summary": operation.Summary, "description": boundedString(operation.Description, 500), "tags": operation.Tags, "risk_level": methodRisk(operation.Method), "requires_approval": operation.Method != http.MethodGet, "path_parameters": operation.PathParams, "query_parameters": operation.QueryParams, "request_body_schema": operation.Body})
	}
	return result
}

var wordPattern = regexp.MustCompile(`[\p{L}\p{N}_/-]+`)

func tokenize(value string) []string {
	raw := wordPattern.FindAllString(strings.ToLower(value), -1)
	seen := map[string]bool{}
	result := []string{}
	for _, group := range raw {
		for _, token := range strings.FieldsFunc(group, func(r rune) bool { return r == '_' || r == '/' || r == '-' }) {
			if len(token) > 1 && !seen[token] {
				seen[token] = true
				result = append(result, token)
			}
		}
	}
	return result
}

var sensitiveKeys = set("password", "secret", "token", "access_token", "refresh_token", "access_key", "private_key", "ssh_key", "passphrase", "cookie", "authorization", "api_key", "credential", "secret_key")
var safeMarkerKeys = set("secret_provided", "password_provided", "credential_provided")
var normalizeKeyPattern = regexp.MustCompile(`[^a-z0-9]+`)
var sensitiveTextPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
	regexp.MustCompile(`\bAKIA[A-Z0-9]{16}\b`),
	regexp.MustCompile(`(?i)\b(password|passphrase|secret|token|api[ _-]?key|authorization|cookie)\b\s*(?:is|[:=])\s*[^\s,;]+`),
	regexp.MustCompile(`(密码|口令|密钥|令牌|私钥)\s*(?:是|为|[:：=])\s*[^\s,，；;]+`),
}

func normalizeKey(value string) string {
	return strings.Trim(normalizeKeyPattern.ReplaceAllString(strings.ToLower(value), "_"), "_")
}
func sensitiveKey(value string) bool {
	normalized := normalizeKey(value)
	if safeMarkerKeys[normalized] {
		return false
	}
	if sensitiveKeys[normalized] {
		return true
	}
	for key := range sensitiveKeys {
		if strings.HasPrefix(normalized, key+"_") || strings.HasSuffix(normalized, "_"+key) {
			return true
		}
	}
	return false
}
func containsSensitive(value any) bool {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			if sensitiveKey(key) || containsSensitive(child) {
				return true
			}
		}
	case []any:
		for _, child := range item {
			if containsSensitive(child) {
				return true
			}
		}
	}
	return false
}
func sanitize(value any, depth int) any {
	if depth > 10 {
		return "[TRUNCATED]"
	}
	switch item := value.(type) {
	case map[string]any:
		result := map[string]any{}
		for key, child := range item {
			if sensitiveKey(key) {
				result[key] = "[REDACTED]"
			} else {
				result[key] = sanitize(child, depth+1)
			}
		}
		return result
	case []any:
		limit := len(item)
		if limit > 100 {
			limit = 100
		}
		result := make([]any, limit)
		for index := 0; index < limit; index++ {
			result[index] = sanitize(item[index], depth+1)
		}
		return result
	case string:
		return boundedString(sanitizeText(item), 4096)
	default:
		return value
	}
}
func sanitizeText(value string) string {
	result := value
	for _, pattern := range sensitiveTextPatterns {
		result = pattern.ReplaceAllString(result, "[REDACTED]")
	}
	return result
}
func stringValue(value any) string { result, _ := value.(string); return result }
func boolValue(value any) bool     { result, _ := value.(bool); return result }
func boolDefault(value any, fallback bool) bool {
	if result, ok := value.(bool); ok {
		return result
	}
	return fallback
}
func stringSlice(value any) []string {
	values := anySlice(value)
	result := make([]string, 0, len(values))
	for _, item := range values {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}
func anySlice(value any) []any { result, _ := value.([]any); return result }
func boundedString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
func min64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
