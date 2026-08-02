package secret

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/asher6312/unapid/internal/buildinfo"
)

var pattern = regexp.MustCompile(`^sk-proj-[A-Za-z0-9_-]{43}$`)

func Validate(value string) (string, error) {
	key := strings.TrimSpace(value)
	if !pattern.MatchString(key) {
		return "", errors.New("the gateway API key is invalid")
	}
	return key, nil
}

func LoadOrCreate() (string, error) {
	contents, err := os.ReadFile(filepath.Join(buildinfo.StateRoot, "api-key"))
	if err == nil {
		if key, validErr := Validate(string(contents)); validErr == nil {
			return key, nil
		}
	}
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return "", errors.New("a secure gateway API key could not be generated")
	}
	return Validate("sk-proj-" + base64.RawURLEncoding.EncodeToString(entropy))
}
