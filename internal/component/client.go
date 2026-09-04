package component

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jumpserver-dev/sdk-go/httplib"
	kaelmodel "github.com/jumpserver/kael/internal/model"
	"go.uber.org/zap"
)

const (
	componentType      = "kael"
	registerPath       = "/api/v1/terminal/terminal-registrations/"
	profilePath        = "/api/v1/users/profile/"
	terminalConfigPath = "/api/v1/terminal/terminals/config/"
	heartbeatPath      = "/api/v1/terminal/terminals/status/"
)

var errInvalidAccessKey = errors.New("component access key file is invalid")

type Options struct {
	CoreURL        string
	TLSVerify      bool
	Timeout        time.Duration
	Name           string
	BootstrapToken string
	AccessKeyFile  string
}

type Client struct {
	mu     sync.Mutex
	client *httplib.Client
}

type accessKey struct {
	ID     string
	Secret string
}

type registrationResponse struct {
	ServiceAccount struct {
		AccessKey accessKey `json:"access_key"`
	} `json:"service_account"`
}

type terminalConfig struct {
	ChatAIEnabled  *bool  `json:"CHAT_AI_ENABLED"`
	ChatAIMethod   string `json:"CHAT_AI_METHOD"`
	ChatAIProvider string `json:"CHAT_AI_PROVIDER"`
	ChatAIBaseURL  string `json:"CHAT_AI_BASE_URL"`
	ChatAIAPIKey   string `json:"CHAT_AI_API_KEY"`
	ChatAIProxy    string `json:"CHAT_AI_PROXY"`
	ChatAIModel    string `json:"CHAT_AI_MODEL"`
}

func Connect(options Options) (*Client, error) {
	if strings.TrimSpace(options.Name) == "" || strings.TrimSpace(options.AccessKeyFile) == "" {
		return nil, fmt.Errorf("component identity is incomplete")
	}
	if options.Timeout < 30*time.Second {
		options.Timeout = 30 * time.Second
	}
	if err := os.MkdirAll(filepath.Dir(options.AccessKeyFile), 0o700); err != nil {
		return nil, fmt.Errorf("create component key directory: %w", err)
	}
	key, err := loadAccessKey(options.AccessKeyFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errInvalidAccessKey) {
		return nil, err
	}
	if err == nil {
		client, clientErr := authenticatedClient(options, key)
		if clientErr != nil {
			return nil, clientErr
		}
		if valid, validationErr := validateAccessKey(client); validationErr == nil && valid {
			return &Client{client: client}, nil
		} else if validationErr != nil {
			return nil, validationErr
		}
	}
	if strings.TrimSpace(options.BootstrapToken) == "" {
		return nil, fmt.Errorf("component access key is missing or unauthorized; set BOOTSTRAP_TOKEN to register Kael")
	}
	key, err = register(options)
	if err != nil {
		return nil, err
	}
	if err = saveAccessKey(options.AccessKeyFile, key); err != nil {
		return nil, err
	}
	client, err := authenticatedClient(options, key)
	if err != nil {
		return nil, err
	}
	return &Client{client: client}, nil
}

func register(options Options) (accessKey, error) {
	client, err := newHTTPClient(options)
	if err != nil {
		return accessKey{}, err
	}
	client.SetAuthSign(&httplib.CustomAuth{AuthScheme: "BootstrapToken", Token: options.BootstrapToken})
	request := map[string]string{"name": options.Name, "comment": componentType, "type": componentType}
	var response registrationResponse
	httpResponse, err := client.Post(registerPath, request, &response)
	if err != nil {
		return accessKey{}, responseError("register Kael component", httpResponse, err)
	}
	key := response.ServiceAccount.AccessKey
	if key.ID == "" || key.Secret == "" {
		return accessKey{}, fmt.Errorf("register Kael component: Core returned an empty access key")
	}
	return key, nil
}

func authenticatedClient(options Options, key accessKey) (*httplib.Client, error) {
	client, err := newHTTPClient(options)
	if err != nil {
		return nil, err
	}
	client.SetHeader("X-JMS-ORG", "ROOT")
	client.SetAuthSign(&httplib.SigAuth{KeyID: key.ID, SecretID: key.Secret})
	return client, nil
}

func newHTTPClient(options Options) (*httplib.Client, error) {
	settings := make([]httplib.Opt, 0, 1)
	if !options.TLSVerify {
		settings = append(settings, httplib.WithInsecure())
	}
	client, err := httplib.NewClient(options.CoreURL, options.Timeout, settings...)
	if err != nil {
		return nil, fmt.Errorf("create Core component client: %w", err)
	}
	return client, nil
}

func validateAccessKey(client *httplib.Client) (bool, error) {
	var profile struct {
		ID string `json:"id"`
	}
	response, err := client.Get(profilePath, &profile)
	if response != nil && response.StatusCode == http.StatusUnauthorized {
		return false, nil
	}
	if err != nil {
		return false, responseError("validate Kael component access key", response, err)
	}
	if profile.ID == "" {
		return false, fmt.Errorf("validate Kael component access key: Core returned an empty profile")
	}
	return true, nil
}

func (c *Client) ModelConfig(ctx context.Context) (kaelmodel.HTTPConfig, error) {
	if err := ctx.Err(); err != nil {
		return kaelmodel.HTTPConfig{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var value terminalConfig
	response, err := c.client.Get(terminalConfigPath, &value)
	if err != nil {
		return kaelmodel.HTTPConfig{}, responseError("load model configuration from TerminalConfig", response, err)
	}
	return modelConfig(value)
}

func modelConfig(value terminalConfig) (kaelmodel.HTTPConfig, error) {
	if value.ChatAIEnabled == nil || !*value.ChatAIEnabled || !strings.EqualFold(strings.TrimSpace(value.ChatAIMethod), "api") {
		return kaelmodel.HTTPConfig{}, fmt.Errorf("Chat AI API is disabled in TerminalConfig")
	}
	config := kaelmodel.HTTPConfig{
		Provider:        strings.ToLower(strings.TrimSpace(value.ChatAIProvider)),
		BaseURL:         strings.TrimSpace(value.ChatAIBaseURL),
		APIKey:          strings.TrimSpace(value.ChatAIAPIKey),
		Model:           strings.TrimSpace(value.ChatAIModel),
		Proxy:           strings.TrimSpace(value.ChatAIProxy),
		ReasoningEffort: "medium",
		Timeout:         5 * time.Minute,
	}
	if config.Provider == "" {
		config.Provider = "openai_compatible"
	}
	if config.BaseURL == "" || config.APIKey == "" || config.Model == "" {
		return kaelmodel.HTTPConfig{}, fmt.Errorf("model endpoint is incomplete in TerminalConfig")
	}
	return config, nil
}

func (c *Client) Heartbeat(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	payload := map[string]any{"sessions": []string{}, "session_online": 0, "cpu_load": 0, "memory_used": 0, "disk_used": 0}
	var tasks []map[string]any
	response, err := c.client.Post(heartbeatPath, payload, &tasks)
	if err != nil {
		return responseError("send Kael component heartbeat", response, err)
	}
	return nil
}

func (c *Client) RunHeartbeat(ctx context.Context, logger *zap.Logger) {
	if logger == nil {
		logger = zap.NewNop()
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := c.Heartbeat(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("Kael component heartbeat failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func loadAccessKey(path string) (accessKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return accessKey{}, err
	}
	parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return accessKey{}, errInvalidAccessKey
	}
	return accessKey{ID: parts[0], Secret: parts[1]}, nil
}

func saveAccessKey(path string, key accessKey) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".access_key-")
	if err != nil {
		return fmt.Errorf("create component access key: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.WriteString(key.ID + ":" + key.Secret)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write component access key: %w", err)
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace component access key: %w", err)
	}
	return nil
}

func responseError(action string, response *http.Response, err error) error {
	if response == nil {
		return fmt.Errorf("%s: Core is unavailable: %w", action, err)
	}
	return fmt.Errorf("%s: Core returned HTTP %d", action, response.StatusCode)
}
