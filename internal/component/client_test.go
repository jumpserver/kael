package component

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestComponentRegistrationModelConfigAndHeartbeat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case registerPath:
			if request.Header.Get("Authorization") != "BootstrapToken bootstrap-secret" {
				t.Errorf("unexpected registration authorization: %s", request.Header.Get("Authorization"))
			}
			var payload map[string]string
			_ = json.NewDecoder(request.Body).Decode(&payload)
			if payload["type"] != componentType || payload["name"] != "kael-test" {
				t.Errorf("unexpected registration payload: %#v", payload)
			}
			_, _ = response.Write([]byte(`{"service_account":{"access_key":{"id":"access-id","secret":"access-secret"}}}`))
		case terminalConfigPath:
			assertSigned(t, request)
			_, _ = response.Write([]byte(`{"CHAT_AI_ENABLED":true,"CHAT_AI_METHOD":"api","CHAT_AI_PROVIDER":"openai_compatible","CHAT_AI_BASE_URL":"https://model.example.test/v1","CHAT_AI_API_KEY":"model-secret","CHAT_AI_MODEL":"model-1"}`))
		case heartbeatPath:
			assertSigned(t, request)
			_, _ = response.Write([]byte(`[]`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	keyPath := filepath.Join(t.TempDir(), "keys", ".access_key")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := Connect(Options{CoreURL: server.URL, TLSVerify: true, Timeout: 30 * time.Second, Name: "kael-test", BootstrapToken: "bootstrap-secret", AccessKeyFile: keyPath})
	if err != nil {
		t.Fatal(err)
	}
	config, err := client.ModelConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.Provider != "openai_compatible" || config.BaseURL != "https://model.example.test/v1" || config.Model != "model-1" || config.APIKey != "model-secret" {
		t.Fatalf("unexpected model config: %#v", config)
	}
	if err = client.Heartbeat(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("access key was not stored privately: %#o", info.Mode().Perm())
	}
}

func TestComponentReusesValidAccessKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.URL.Path != profilePath {
			t.Errorf("unexpected request path: %s", request.URL.Path)
			http.NotFound(response, request)
			return
		}
		assertSigned(t, request)
		_, _ = response.Write([]byte(`{"id":"service-account-id"}`))
	}))
	defer server.Close()

	keyPath := filepath.Join(t.TempDir(), ".access_key")
	if err := os.WriteFile(keyPath, []byte("access-id:access-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Connect(Options{CoreURL: server.URL, TLSVerify: true, Name: "kael-test", AccessKeyFile: keyPath}); err != nil {
		t.Fatal(err)
	}
}

func assertSigned(t *testing.T, request *http.Request) {
	t.Helper()
	if !strings.HasPrefix(request.Header.Get("Authorization"), "Signature ") || request.Header.Get("X-JMS-ORG") != "ROOT" {
		t.Errorf("request was not signed as a component: authorization=%q org=%q", request.Header.Get("Authorization"), request.Header.Get("X-JMS-ORG"))
	}
}
