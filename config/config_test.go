package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvDoesNotOverrideProcessEnvironment(t *testing.T) {
	t.Setenv("WS_AGENT_DOTENV_TEST", "from-process")
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("WS_AGENT_DOTENV_TEST=from-file\nWS_AGENT_DOTENV_NEW='loaded'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_ = os.Unsetenv("WS_AGENT_DOTENV_NEW")
	t.Cleanup(func() { _ = os.Unsetenv("WS_AGENT_DOTENV_NEW") })
	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("WS_AGENT_DOTENV_TEST"); got != "from-process" {
		t.Fatalf("existing value overwritten: %q", got)
	}
	if got := os.Getenv("WS_AGENT_DOTENV_NEW"); got != "loaded" {
		t.Fatalf("new value = %q", got)
	}
}

func TestParseBool(t *testing.T) {
	for _, value := range []string{"true", "TRUE", "1"} {
		if !parseBool(value) {
			t.Errorf("parseBool(%q) = false", value)
		}
	}
	for _, value := range []string{"", "false", "yes", "invalid"} {
		if parseBool(value) {
			t.Errorf("parseBool(%q) = true", value)
		}
	}
}
