package material

import (
	"strings"
	"testing"
)

func TestFilesRenderIsolatedTwoServiceRuntime(t *testing.T) {
	files, err := Files("n8n_default")
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string]string{}
	for _, file := range files {
		contents[file.Name] = string(file.Data)
	}
	compose := contents["compose.yaml"]
	for _, expected := range []string{
		"translator:",
		"api:",
		"subscription-api-gateway",
		`- "8317"`,
		"internal: true",
		"egress:",
		"name: n8n_default",
		"../state/codex",
		"../state/api-key",
	} {
		if !strings.Contains(compose, expected) {
			t.Fatalf("Compose output missing %q", expected)
		}
	}
	if strings.Contains(compose, "ports:") {
		t.Fatal("Compose output published a host port")
	}
	if !strings.Contains(contents[".dockerignore"], "credentials") || !strings.Contains(contents[".dockerignore"], "secrets") {
		t.Fatal("Docker build context does not exclude credentials")
	}
}

func TestFilesRejectYAMLInjection(t *testing.T) {
	if _, err := Files("n8n_default\nports:"); err == nil {
		t.Fatal("unsafe network name was accepted")
	}
}
