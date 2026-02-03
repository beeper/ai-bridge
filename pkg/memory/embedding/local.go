package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const DefaultLocalEmbeddingModel = "text-embedding-3-small"

func NewLocalProvider(baseURL, apiKey, model string, headers map[string]string) (*Provider, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("local embeddings require base_url")
	}
	normalizedModel := strings.TrimSpace(model)
	if normalizedModel == "" {
		normalizedModel = DefaultLocalEmbeddingModel
	}
	endpoint := normalizeOpenAIEndpoint(baseURL)

	embedBatch := func(ctx context.Context, texts []string) ([][]float64, error) {
		if len(texts) == 0 {
			return nil, nil
		}
		payload := map[string]any{
			"model": normalizedModel,
			"input": texts,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(apiKey) != "" {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
		}
		for key, value := range headers {
			if strings.TrimSpace(value) == "" {
				continue
			}
			req.Header.Set(key, value)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("local embeddings failed: %s %s", resp.Status, string(data))
		}
		var payloadResp struct {
			Data []struct {
				Embedding []float64 `json:"embedding"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &payloadResp); err != nil {
			return nil, err
		}
		out := make([][]float64, 0, len(payloadResp.Data))
		for _, entry := range payloadResp.Data {
			out = append(out, NormalizeEmbedding(entry.Embedding))
		}
		return out, nil
	}

	return &Provider{
		id:    "local",
		model: normalizedModel,
		embedQuery: func(ctx context.Context, text string) ([]float64, error) {
			results, err := embedBatch(ctx, []string{text})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
		embedBatch: embedBatch,
	}, nil
}

func normalizeOpenAIEndpoint(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(trimmed, "/embeddings") {
		return trimmed
	}
	if strings.HasSuffix(trimmed, "/v1") || strings.HasSuffix(trimmed, "/openai/v1") {
		return trimmed + "/embeddings"
	}
	return trimmed + "/v1/embeddings"
}
