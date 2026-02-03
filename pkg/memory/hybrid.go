package memory

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

var tokenRE = regexp.MustCompile(`[A-Za-z0-9_]+`)

// BuildFtsQuery builds a simple AND query for FTS5 from raw input.
func BuildFtsQuery(raw string) string {
	tokens := tokenRE.FindAllString(raw, -1)
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		clean := strings.ReplaceAll(token, `"`, "")
		parts = append(parts, `"`+clean+`"`)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " AND ")
}

// BM25RankToScore normalizes an FTS5 bm25 rank into a 0-1-ish score.
func BM25RankToScore(rank float64) float64 {
	if !isFinite(rank) {
		return 1 / (1 + 999)
	}
	if rank < 0 {
		rank = 0
	}
	return 1 / (1 + rank)
}

type HybridVectorResult struct {
	ID          string
	Path        string
	StartLine   int
	EndLine     int
	Source      string
	Snippet     string
	VectorScore float64
}

type HybridKeywordResult struct {
	ID        string
	Path      string
	StartLine int
	EndLine   int
	Source    string
	Snippet   string
	TextScore float64
}

// MergeHybridResults merges vector + keyword results with weighted scores.
func MergeHybridResults(vector []HybridVectorResult, keyword []HybridKeywordResult, vectorWeight, textWeight float64) []SearchResult {
	type entry struct {
		id          string
		path        string
		startLine   int
		endLine     int
		source      string
		snippet     string
		vectorScore float64
		textScore   float64
	}
	byID := make(map[string]*entry)

	for _, r := range vector {
		byID[r.ID] = &entry{
			id:          r.ID,
			path:        r.Path,
			startLine:   r.StartLine,
			endLine:     r.EndLine,
			source:      r.Source,
			snippet:     r.Snippet,
			vectorScore: r.VectorScore,
		}
	}

	for _, r := range keyword {
		if existing, ok := byID[r.ID]; ok {
			existing.textScore = r.TextScore
			if r.Snippet != "" {
				existing.snippet = r.Snippet
			}
			continue
		}
		byID[r.ID] = &entry{
			id:        r.ID,
			path:      r.Path,
			startLine: r.StartLine,
			endLine:   r.EndLine,
			source:    r.Source,
			snippet:   r.Snippet,
			textScore: r.TextScore,
		}
	}

	results := make([]SearchResult, 0, len(byID))
	for _, e := range byID {
		score := vectorWeight*e.vectorScore + textWeight*e.textScore
		results = append(results, SearchResult{
			Path:      e.path,
			StartLine: e.startLine,
			EndLine:   e.endLine,
			Score:     score,
			Snippet:   e.snippet,
			Source:    e.source,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
