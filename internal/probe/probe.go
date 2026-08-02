package probe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/asher6312/unapid/internal/secret"
)

const maxResponseBytes = 1024 * 1024

func Fetch(ctx context.Context, endpoint, keyFile string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("the probe URL is invalid")
	}
	if keyFile != "" {
		contents, readErr := os.ReadFile(keyFile)
		if readErr != nil {
			return nil, errors.New("the probe API key could not be read")
		}
		key, keyErr := secret.Validate(string(contents))
		if keyErr != nil {
			return nil, keyErr
		}
		request.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("the API gateway did not answer")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return nil, errors.New("the API gateway returned an unreadable response")
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		detail := strings.TrimSpace(string(body))
		if len(detail) > 300 {
			detail = detail[len(detail)-300:]
		}
		return nil, fmt.Errorf("the API gateway returned HTTP %d: %s", response.StatusCode, detail)
	}
	return body, nil
}
