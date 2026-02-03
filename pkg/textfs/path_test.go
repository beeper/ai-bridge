package textfs

import "testing"

func TestIsMemoryPath(t *testing.T) {
	cases := map[string]bool{
		"MEMORY.md":             true,
		"memory.md":             true,
		"memory/2024-01.md":     true,
		"memory/nested/file.md": true,
		"notes.md":              false,
		"memory.txt":            false,
		"memory/notes.txt":      true,
	}
	for path, want := range cases {
		got := IsMemoryPath(path)
		if got != want {
			t.Fatalf("IsMemoryPath(%q) = %v, want %v", path, got, want)
		}
	}
}
