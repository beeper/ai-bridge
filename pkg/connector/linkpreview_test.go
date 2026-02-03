package connector

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
)

func TestPreviewCacheReturnsClones(t *testing.T) {
	cache := &previewCache{
		entries: make(map[string]*previewCacheEntry),
	}

	orig := &PreviewWithImage{
		Preview: &event.BeeperLinkPreview{
			LinkPreview: event.LinkPreview{
				Title: "original",
			},
		},
		ImageData: []byte{1, 2},
		ImageURL:  "https://example.com/image.png",
	}

	cache.set("https://example.com", orig, time.Hour)

	// Mutate original after caching; cache should be isolated.
	orig.Preview.Title = "mutated"
	orig.ImageData[0] = 9

	first := cache.get("https://example.com")
	if first == nil || first.Preview == nil {
		t.Fatal("expected cached preview")
	}
	if first.Preview.Title != "original" {
		t.Fatalf("expected original title, got %q", first.Preview.Title)
	}
	if len(first.ImageData) != 2 || first.ImageData[0] != 1 {
		t.Fatalf("expected original image data, got %v", first.ImageData)
	}

	// Mutate returned copy; subsequent fetch should be unaffected.
	first.Preview.Title = "changed"
	first.ImageData[0] = 7

	second := cache.get("https://example.com")
	if second == nil || second.Preview == nil {
		t.Fatal("expected cached preview")
	}
	if second.Preview.Title != "original" {
		t.Fatalf("expected original title on second fetch, got %q", second.Preview.Title)
	}
	if len(second.ImageData) != 2 || second.ImageData[0] != 1 {
		t.Fatalf("expected original image data on second fetch, got %v", second.ImageData)
	}
}
