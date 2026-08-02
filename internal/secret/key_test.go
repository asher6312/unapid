package secret

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateConventionalKey(t *testing.T) {
	key := "sk-proj-" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if got, err := Validate(key); err != nil || got != key {
		t.Fatalf("Validate() = %q, %v", got, err)
	}
	for _, invalid := range []string{"", "local-only", "sk-proj-short", key + "x"} {
		if _, err := Validate(invalid); err == nil {
			t.Fatalf("invalid key accepted: %q", invalid)
		}
	}
	if strings.Contains(key, "=") {
		t.Fatal("key is not raw URL-safe base64")
	}
}
