package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	CodexBinary            string
	Name                   string
	CoreHost               string
	BootstrapToken         string
	BindHost               string
	HTTPPort               int
	IgnoreVerifyCerts      bool
	HTTPRequestTimeout     time.Duration
	ToolResultTimeout      time.Duration
	AllowedOrigins         []string
	TrustForwardedHeaders  bool
	AccessKeyFilePath      string
	ArtifactFolderPath     string
	RuntimeDataFolderPath  string
	RuntimeStore           string
	PlatformGatewayEnabled bool
	PlatformDelegationKey  string
	PlatformDelegationID   string
	PlatformIssuer         string
	PlatformAudience       string
	PlatformCACert         string
	PlatformClientCert     string
	PlatformClientKey      string
	PlatformAllowedMethods map[string]bool
	PlatformRegistryTTL    time.Duration
	PlatformTimeout        time.Duration
	PlatformMaxResponse    int64
}

func Load(path string) (Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	setDefaults(v)
	if err := readConfig(v, path); err != nil {
		return Config{}, err
	}
	for _, section := range []string{"core", "identity", "artifact", "runtime", "platform_gateway"} {
		if v.InConfig(section) {
			return Config{}, fmt.Errorf("nested config section %q is not supported; use flat Koko-style keys", section)
		}
	}
	root, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("resolve working directory: %w", err)
	}
	dataFolder := filepath.Join(root, "data")
	result := Config{
		CodexBinary:            strings.TrimSpace(v.GetString("CODEX_BINARY")),
		Name:                   strings.TrimSpace(v.GetString("NAME")),
		CoreHost:               strings.TrimRight(strings.TrimSpace(v.GetString("CORE_HOST")), "/"),
		BootstrapToken:         strings.TrimSpace(v.GetString("BOOTSTRAP_TOKEN")),
		BindHost:               strings.TrimSpace(v.GetString("BIND_HOST")),
		HTTPPort:               v.GetInt("HTTPD_PORT"),
		IgnoreVerifyCerts:      v.GetBool("IGNORE_VERIFY_CERTS"),
		HTTPRequestTimeout:     time.Duration(v.GetInt("HTTP_REQUEST_TIMEOUT")) * time.Second,
		ToolResultTimeout:      v.GetDuration("TOOL_RESULT_TIMEOUT"),
		AllowedOrigins:         v.GetStringSlice("ALLOWED_ORIGINS"),
		TrustForwardedHeaders:  v.GetBool("TRUST_FORWARDED_HEADERS"),
		AccessKeyFilePath:      filepath.Join(dataFolder, "keys", ".access_key"),
		ArtifactFolderPath:     filepath.Join(dataFolder, "artifacts"),
		RuntimeDataFolderPath:  dataFolder,
		RuntimeStore:           strings.ToLower(strings.TrimSpace(v.GetString("RUNTIME_STORE"))),
		PlatformGatewayEnabled: v.GetBool("PLATFORM_GATEWAY_ENABLED"),
		PlatformDelegationKey:  strings.TrimSpace(v.GetString("PLATFORM_DELEGATION_KEY")),
		PlatformDelegationID:   strings.TrimSpace(v.GetString("PLATFORM_DELEGATION_KEY_ID")),
		PlatformIssuer:         strings.TrimSpace(v.GetString("PLATFORM_DELEGATION_ISSUER")),
		PlatformAudience:       strings.TrimSpace(v.GetString("PLATFORM_DELEGATION_AUDIENCE")),
		PlatformCACert:         v.GetString("PLATFORM_CA_CERT"),
		PlatformClientCert:     v.GetString("PLATFORM_CLIENT_CERT"),
		PlatformClientKey:      v.GetString("PLATFORM_CLIENT_KEY"),
		PlatformAllowedMethods: make(map[string]bool),
		PlatformRegistryTTL:    v.GetDuration("PLATFORM_REGISTRY_TTL"),
		PlatformTimeout:        v.GetDuration("PLATFORM_TIMEOUT"),
		PlatformMaxResponse:    v.GetInt64("PLATFORM_MAX_RESPONSE_BYTES"),
	}
	for _, method := range v.GetStringSlice("PLATFORM_ALLOWED_METHODS") {
		result.PlatformAllowedMethods[strings.ToUpper(strings.TrimSpace(method))] = true
	}
	if err = result.Validate(); err != nil {
		return Config{}, err
	}
	return result, nil
}

func readConfig(v *viper.Viper, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("KAEL_CONFIG_FILE"))
	}
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("read config %q: %w", path, err)
		}
		return nil
	}
	for _, candidate := range []string{"config.yml", "config.yaml", ".config.yml", ".config.yaml"} {
		if _, err := os.Stat(candidate); err == nil {
			v.SetConfigFile(candidate)
			if err = v.ReadInConfig(); err != nil {
				return fmt.Errorf("read config %q: %w", candidate, err)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect config %q: %w", candidate, err)
		}
	}
	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("CODEX_BINARY", "codex")
	v.SetDefault("NAME", defaultName())
	v.SetDefault("CORE_HOST", "http://127.0.0.1:8080")
	v.SetDefault("BIND_HOST", "0.0.0.0")
	v.SetDefault("HTTPD_PORT", 8083)
	v.SetDefault("HTTP_REQUEST_TIMEOUT", 30)
	v.SetDefault("TOOL_RESULT_TIMEOUT", "45s")
	v.SetDefault("RUNTIME_STORE", "core")
	v.SetDefault("PLATFORM_GATEWAY_ENABLED", true)
	v.SetDefault("PLATFORM_DELEGATION_KEY_ID", "v1")
	v.SetDefault("PLATFORM_DELEGATION_ISSUER", "jumpserver-ai")
	v.SetDefault("PLATFORM_DELEGATION_AUDIENCE", "jumpserver-core")
	v.SetDefault("PLATFORM_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "PATCH"})
	v.SetDefault("PLATFORM_REGISTRY_TTL", "1h")
	v.SetDefault("PLATFORM_TIMEOUT", "15s")
	v.SetDefault("PLATFORM_MAX_RESPONSE_BYTES", 1024*1024)
}

func defaultName() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "localhost"
	}
	value := []rune("[Kael]-" + hostname)
	if len(value) > 128 {
		value = value[:128]
	}
	return string(value)
}

func (c Config) Validate() error {
	if c.ToolResultTimeout < time.Second || c.ToolResultTimeout > 10*time.Minute {
		return fmt.Errorf("TOOL_RESULT_TIMEOUT must be between 1s and 10m")
	}
	if c.Name == "" || c.BindHost == "" || c.HTTPPort < 1 || c.HTTPPort > 65535 {
		return fmt.Errorf("component name or listen address is invalid")
	}
	if !validEndpoint(c.CoreHost) {
		return fmt.Errorf("CORE_HOST must be an HTTP/HTTPS endpoint without embedded credentials")
	}
	if c.HTTPRequestTimeout < time.Second || c.HTTPRequestTimeout > 5*time.Minute {
		return fmt.Errorf("HTTP_REQUEST_TIMEOUT must be between 1 and 300 seconds")
	}
	for _, origin := range c.AllowedOrigins {
		if !validOrigin(origin) {
			return fmt.Errorf("ALLOWED_ORIGINS contains an invalid HTTP/HTTPS origin")
		}
	}
	if c.RuntimeStore != "core" && c.RuntimeStore != "jsonl" {
		return fmt.Errorf("RUNTIME_STORE must be core or jsonl")
	}
	if !c.PlatformGatewayEnabled {
		return fmt.Errorf("PLATFORM_GATEWAY_ENABLED must be true because the default general assistant requires the Platform Gateway")
	}
	if len(c.PlatformDelegationKey) < 32 {
		return fmt.Errorf("PLATFORM_DELEGATION_KEY must contain at least 32 characters after trimming surrounding whitespace and match Core")
	}
	if c.PlatformDelegationID == "" || c.PlatformIssuer == "" || c.PlatformAudience == "" || c.PlatformRegistryTTL <= 0 || c.PlatformTimeout <= 0 || c.PlatformMaxResponse < 1 {
		return fmt.Errorf("Platform Gateway configuration is incomplete")
	}
	if (c.PlatformClientCert == "") != (c.PlatformClientKey == "") {
		return fmt.Errorf("PLATFORM_CLIENT_CERT and PLATFORM_CLIENT_KEY must be configured together")
	}
	return nil
}

func validEndpoint(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func validOrigin(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.User == nil && (parsed.Path == "" || parsed.Path == "/") && parsed.RawQuery == "" && parsed.Fragment == ""
}
