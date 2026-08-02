package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchReadsSuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	body, err := Fetch(context.Background(), server.URL, "")
	if err != nil || string(body) != `{"status":"ok"}` {
		t.Fatalf("Fetch() = %s, %v", body, err)
	}
}

func TestFetchRejectsFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Error(response, "no", http.StatusBadGateway)
	}))
	defer server.Close()
	if _, err := Fetch(context.Background(), server.URL, ""); err == nil {
		t.Fatal("failed HTTP response was accepted")
	}
}
