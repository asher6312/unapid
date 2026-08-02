package proxy

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/asher6312/unapid/internal/secret"
)

type HealthCheck func(context.Context) error

func writeJSON(response http.ResponseWriter, status int, payload any) {
	body, _ := json.Marshal(payload)
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	response.WriteHeader(status)
	_, _ = response.Write(body)
}

func authorized(header, expected string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	supplied := []byte(strings.TrimPrefix(header, prefix))
	wanted := []byte(expected)
	return len(supplied) == len(wanted) && subtle.ConstantTimeCompare(supplied, wanted) == 1
}

func NewHandler(apiKey string, target *url.URL, health HealthCheck) (http.Handler, error) {
	if _, err := secret.Validate(apiKey); err != nil {
		return nil, err
	}
	if target == nil || target.Scheme != "http" || target.Host == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		return nil, errors.New("the OAuth translator URL is invalid")
	}
	reverse := httputil.NewSingleHostReverseProxy(target)
	originalDirector := reverse.Director
	reverse.Director = func(request *http.Request) {
		originalDirector(request)
		request.Header.Del("Authorization")
		request.Header.Del("Proxy-Authorization")
	}
	reverse.ErrorHandler = func(response http.ResponseWriter, _ *http.Request, _ error) {
		writeJSON(response, http.StatusBadGateway, map[string]any{
			"error": map[string]string{
				"message": "The upstream service is unavailable.",
				"type":    "api_error",
				"code":    "upstream_unavailable",
			},
		})
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			if health != nil {
				ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
				err := health(ctx)
				cancel()
				if err != nil {
					writeJSON(response, http.StatusServiceUnavailable, map[string]string{"status": "starting"})
					return
				}
			}
			writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		if !authorized(request.Header.Get("Authorization"), apiKey) {
			writeJSON(response, http.StatusUnauthorized, map[string]any{
				"error": map[string]string{
					"message": "Invalid API key.",
					"type":    "invalid_request_error",
					"code":    "invalid_api_key",
				},
			})
			return
		}
		reverse.ServeHTTP(response, request)
	}), nil
}

func Run(ctx context.Context, listenAddress, upstream, keyFile string) error {
	contents, err := os.ReadFile(keyFile)
	if err != nil {
		return errors.New("the gateway API key could not be read")
	}
	apiKey, err := secret.Validate(string(contents))
	if err != nil {
		return err
	}
	target, err := url.Parse(upstream)
	if err != nil {
		return errors.New("the OAuth translator URL is invalid")
	}
	health := func(ctx context.Context) error {
		connection, dialErr := (&net.Dialer{}).DialContext(ctx, "tcp", target.Host)
		if dialErr != nil {
			return dialErr
		}
		return connection.Close()
	}
	handler, err := NewHandler(apiKey, target, health)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              listenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case serveErr := <-errChannel:
		if errors.Is(serveErr, http.ErrServerClosed) {
			return nil
		}
		return serveErr
	}
}
