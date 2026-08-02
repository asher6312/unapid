package proxy

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func fixtureKey() string {
	return "sk-proj-" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
}

func TestHandlerRequiresKeyAndStripsItUpstream(t *testing.T) {
	var upstreamAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		upstreamAuthorization = request.Header.Get("Authorization")
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"data":[{"id":"gpt-test"}]}`)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	handler, err := NewHandler(fixtureKey(), target, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if unauthorized.Code != http.StatusUnauthorized || !strings.Contains(unauthorized.Body.String(), "invalid_api_key") {
		t.Fatalf("unexpected unauthorized response: %d %s", unauthorized.Code, unauthorized.Body.String())
	}

	authorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.Header.Set("Authorization", "Bearer "+fixtureKey())
	handler.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusOK || upstreamAuthorization != "" {
		t.Fatalf("proxy status=%d upstream auth=%q", authorized.Code, upstreamAuthorization)
	}
}

func TestHealthIsPublicButChecksTranslator(t *testing.T) {
	target, _ := url.Parse("http://translator:8318")
	handler, err := NewHandler(fixtureKey(), target, func(context.Context) error { return context.DeadlineExceeded })
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("health status=%d, want 503", response.Code)
	}
}
