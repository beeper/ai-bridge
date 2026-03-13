package websearch

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/beeper/agentremote/pkg/search"
)

type PayloadResult struct {
	ID          string
	Title       string
	URL         string
	Description string
	Published   string
	SiteName    string
	Author      string
	Image       string
	Favicon     string
}

// RequestFromArgs converts tool arguments into a normalized search request.
func RequestFromArgs(args map[string]any) (search.Request, error) {
	query, ok := args["query"].(string)
	if !ok {
		return search.Request{}, errors.New("missing or invalid 'query' argument")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return search.Request{}, errors.New("missing or invalid 'query' argument")
	}
	count, _ := ParseCountAndIgnoredOptions(args)
	country, _ := args["country"].(string)
	searchLang, _ := args["search_lang"].(string)
	uiLang, _ := args["ui_lang"].(string)
	freshness, _ := args["freshness"].(string)

	return search.Request{
		Query:      query,
		Count:      count,
		Country:    strings.TrimSpace(country),
		SearchLang: strings.TrimSpace(searchLang),
		UILang:     strings.TrimSpace(uiLang),
		Freshness:  strings.TrimSpace(freshness),
	}, nil
}

// PayloadFromResponse converts a normalized search response into the common JSON payload shape.
func PayloadFromResponse(resp *search.Response) map[string]any {
	payload := map[string]any{
		"query":      resp.Query,
		"provider":   resp.Provider,
		"count":      resp.Count,
		"tookMs":     resp.TookMs,
		"answer":     resp.Answer,
		"summary":    resp.Summary,
		"definition": resp.Definition,
		"warning":    resp.Warning,
		"noResults":  resp.NoResults,
		"cached":     resp.Cached,
	}

	if len(resp.Results) > 0 {
		results := make([]map[string]any, 0, len(resp.Results))
		for _, r := range resp.Results {
			entry := map[string]any{
				"title":       r.Title,
				"url":         r.URL,
				"description": r.Description,
				"published":   r.Published,
				"siteName":    r.SiteName,
			}
			if r.ID != "" {
				entry["id"] = r.ID
			}
			if r.Author != "" {
				entry["author"] = r.Author
			}
			if r.Image != "" {
				entry["image"] = r.Image
			}
			if r.Favicon != "" {
				entry["favicon"] = r.Favicon
			}
			results = append(results, entry)
		}
		payload["results"] = results
	}

	if resp.Extras != nil {
		payload["extras"] = resp.Extras
	}
	return payload
}

// ResultsFromPayload extracts search results from the common payload map.
func ResultsFromPayload(payload map[string]any) []PayloadResult {
	rawResults, ok := payload["results"].([]any)
	if !ok || len(rawResults) == 0 {
		return nil
	}

	results := make([]PayloadResult, 0, len(rawResults))
	for _, rawResult := range rawResults {
		entry, ok := rawResult.(map[string]any)
		if !ok {
			continue
		}
		results = append(results, PayloadResult{
			ID:          stringArg(entry, "id"),
			Title:       stringArg(entry, "title"),
			URL:         stringArg(entry, "url"),
			Description: stringArg(entry, "description"),
			Published:   stringArg(entry, "published"),
			SiteName:    stringArg(entry, "siteName"),
			Author:      stringArg(entry, "author"),
			Image:       stringArg(entry, "image"),
			Favicon:     stringArg(entry, "favicon"),
		})
	}
	return results
}

// ResultsFromJSON extracts search results from a JSON-encoded payload.
func ResultsFromJSON(output string) []PayloadResult {
	output = strings.TrimSpace(output)
	if output == "" || !strings.HasPrefix(output, "{") {
		return nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return nil
	}
	return ResultsFromPayload(payload)
}

func stringArg(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}
