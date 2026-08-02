package account

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asher6312/unapid/internal/process"
)

var validFixture = []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh"}}`)

type loginRunner struct{}

func (loginRunner) Capture(context.Context, string, ...string) (process.Result, error) {
	return process.Result{}, nil
}

func (loginRunner) Interactive(_ context.Context, env []string, name string, args ...string) error {
	if name == "" || len(args) < 2 || args[0] != "login" || args[1] != "--device-auth" {
		return os.ErrInvalid
	}
	home := envValue(env, "CODEX_HOME")
	if home == "" {
		return os.ErrInvalid
	}
	return os.WriteFile(filepath.Join(home, "auth.json"), validFixture, 0o600)
}

func TestValidateRequiresChatGPTTokens(t *testing.T) {
	if err := Validate(validFixture); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{
		{},
		[]byte(`{"auth_mode":"api"}`),
		[]byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"only"}}`),
	} {
		if Validate(invalid) == nil {
			t.Fatalf("invalid credential accepted: %s", invalid)
		}
	}
}

func TestDeviceLoginUsesIsolatedCodexHome(t *testing.T) {
	home := t.TempDir()
	store := NewWith(loginRunner{}, home, []string{"PATH=/usr/bin"})
	contents, err := store.DeviceLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != string(validFixture) {
		t.Fatal("device login returned unexpected credentials")
	}
	path := filepath.Join(home, ".unapid", "codex", "auth.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("auth mode = %o, want 600", info.Mode().Perm())
	}
}

func TestVersionOrdering(t *testing.T) {
	if !newerVersion("v22.23.1", "v20.19.0") || newerVersion("v20.19.0", "v22.23.1") {
		t.Fatal("semantic Node version ordering failed")
	}
	if _, ok := parseVersion("not-a-version"); ok {
		t.Fatal("invalid version accepted")
	}
}

func TestSetEnvReplacesSensitiveOverrides(t *testing.T) {
	env := setEnv([]string{"PATH=/bin", "CODEX_HOME=/wrong"}, "CODEX_HOME", "/isolated")
	if got := envValue(env, "CODEX_HOME"); got != "/isolated" {
		t.Fatalf("CODEX_HOME = %q", got)
	}
	if strings.Count(strings.Join(env, "\n"), "CODEX_HOME=") != 1 {
		t.Fatal("duplicate CODEX_HOME remained")
	}
}
