package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFlatKokoStyleEnvironment(t *testing.T) {
	t.Setenv("CORE_HOST", "https://core.example.test")
	t.Setenv("BOOTSTRAP_TOKEN", "bootstrap-secret")
	t.Setenv("NAME", "kael-test")
	t.Setenv("BIND_HOST", "127.0.0.1")
	t.Setenv("HTTPD_PORT", "9083")
	t.Setenv("IGNORE_VERIFY_CERTS", "true")
	t.Setenv("HTTP_REQUEST_TIMEOUT", "45")

	settings, err := Load(writeConfig(t, "{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if settings.CoreHost != "https://core.example.test" || settings.BootstrapToken != "bootstrap-secret" || settings.Name != "kael-test" || settings.BindHost != "127.0.0.1" || settings.HTTPPort != 9083 || !settings.IgnoreVerifyCerts || settings.HTTPRequestTimeout != 45*time.Second {
		t.Fatalf("flat environment was not loaded: %#v", settings)
	}
}

func TestLoadDiscoversFlatConfigAndDerivesDataPaths(t *testing.T) {
	dir := t.TempDir()
	content := "CORE_HOST: https://core.example.test\nNAME: kael-test\nHTTPD_PORT: 9083\nPLATFORM_DELEGATION_KEY: test-only-delegation-key-00000000\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KAEL_CONFIG_FILE", "")
	t.Setenv("CORE_HOST", "")
	t.Setenv("NAME", "")
	t.Chdir(dir)

	settings, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if settings.CoreHost != "https://core.example.test" || settings.Name != "kael-test" || settings.HTTPPort != 9083 || settings.AccessKeyFilePath != filepath.Join(dir, "data", "keys", ".access_key") || settings.ArtifactFolderPath != filepath.Join(dir, "data", "artifacts") || settings.RuntimeDataFolderPath != filepath.Join(dir, "data") {
		t.Fatalf("unexpected flat config: %#v", settings)
	}
}

func TestLoadUsesExplicitFlatConfig(t *testing.T) {
	path := writeConfig(t, "CORE_HOST: https://core.example.test\n")
	t.Setenv("KAEL_CONFIG_FILE", path)
	t.Setenv("CORE_HOST", "")

	settings, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if settings.CoreHost != "https://core.example.test" {
		t.Fatalf("unexpected Core host: %s", settings.CoreHost)
	}
}

func TestLoadRejectsNestedConfig(t *testing.T) {
	if _, err := Load(writeConfig(t, "core:\n  url: https://core.example.test\n")); err == nil {
		t.Fatal("nested config was accepted")
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	t.Setenv("PLATFORM_DELEGATION_KEY", "test-only-delegation-key-00000000")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
