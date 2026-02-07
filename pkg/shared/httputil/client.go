package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PostJSON marshals payload as JSON and sends a POST request with the given headers.
// Returns the response body, status code, and any error.
func PostJSON(ctx context.Context, url string, headers map[string]string, payload any, timeoutSecs int) ([]byte, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("http %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

// GetJSON sends a GET request with the given headers and returns the response body.
func GetJSON(ctx context.Context, url string, headers map[string]string, timeoutSecs int) ([]byte, int, error) {
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("http %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}
