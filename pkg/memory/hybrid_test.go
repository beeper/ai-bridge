package memory

import "testing"

func TestBM25RankToScore(t *testing.T) {
	if score := BM25RankToScore(-1); score != 1 {
		t.Fatalf("expected negative rank to clamp to 1, got %v", score)
	}
	if score := BM25RankToScore(0); score != 1 {
		t.Fatalf("expected rank 0 to be 1, got %v", score)
	}
	if score := BM25RankToScore(1); score <= 0.4 || score >= 0.6 {
		t.Fatalf("expected rank 1 to be around 0.5, got %v", score)
	}
}

func TestMergeHybridResults(t *testing.T) {
	vector := []HybridVectorResult{
		{
			ID:          "a",
			Path:        "memory/a.md",
			StartLine:   1,
			EndLine:     2,
			Source:      "memory",
			Snippet:     "Vector A",
			VectorScore: 0.9,
		},
		{
			ID:          "b",
			Path:        "memory/b.md",
			StartLine:   3,
			EndLine:     4,
			Source:      "memory",
			Snippet:     "Vector B",
			VectorScore: 0.2,
		},
	}
	keyword := []HybridKeywordResult{
		{
			ID:        "b",
			Path:      "memory/b.md",
			StartLine: 3,
			EndLine:   4,
			Source:    "memory",
			Snippet:   "Keyword B",
			TextScore: 0.8,
		},
		{
			ID:        "c",
			Path:      "memory/c.md",
			StartLine: 5,
			EndLine:   6,
			Source:    "memory",
			Snippet:   "Keyword C",
			TextScore: 0.7,
		},
	}

	results := MergeHybridResults(vector, keyword, 0.5, 0.5)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Path != "memory/b.md" {
		t.Fatalf("expected top result to be memory/b.md, got %s", results[0].Path)
	}
	if results[1].Path != "memory/a.md" {
		t.Fatalf("expected second result to be memory/a.md, got %s", results[1].Path)
	}
	if results[2].Path != "memory/c.md" {
		t.Fatalf("expected third result to be memory/c.md, got %s", results[2].Path)
	}
	if results[0].Snippet != "Keyword B" {
		t.Fatalf("expected keyword snippet to override vector snippet, got %s", results[0].Snippet)
	}
}
