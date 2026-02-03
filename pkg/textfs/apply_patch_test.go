package textfs

import (
	"context"
	"strings"
	"testing"
)

func TestApplyPatchAddUpdateDeleteMove(t *testing.T) {
	ctx := context.Background()
	db := setupTextfsDB(t)
	store := NewStore(db, "bridge", "login", "agent")

	addPatch := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: notes/a.txt",
		"+hello",
		"+world",
		"*** End Patch",
	}, "\n")

	if _, err := ApplyPatch(store, addPatch); err != nil {
		t.Fatalf("apply add patch: %v", err)
	}
	entry, found, err := store.Read(ctx, "notes/a.txt")
	if err != nil || !found {
		t.Fatalf("read added file: found=%v err=%v", found, err)
	}
	if entry.Content != "hello\nworld\n" {
		t.Fatalf("unexpected content: %q", entry.Content)
	}

	updatePatch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: notes/a.txt",
		"@@",
		"-hello",
		"+hi",
		"*** End Patch",
	}, "\n")

	if _, err := ApplyPatch(store, updatePatch); err != nil {
		t.Fatalf("apply update patch: %v", err)
	}
	entry, found, err = store.Read(ctx, "notes/a.txt")
	if err != nil || !found {
		t.Fatalf("read updated file: found=%v err=%v", found, err)
	}
	if entry.Content != "hi\nworld\n" {
		t.Fatalf("unexpected updated content: %q", entry.Content)
	}

	movePatch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: notes/a.txt",
		"*** Move to: notes/b.txt",
		"@@",
		"-world",
		"+earth",
		"*** End Patch",
	}, "\n")

	if _, err := ApplyPatch(store, movePatch); err != nil {
		t.Fatalf("apply move patch: %v", err)
	}
	_, found, err = store.Read(ctx, "notes/a.txt")
	if err != nil {
		t.Fatalf("read old file after move: %v", err)
	}
	if found {
		t.Fatalf("expected old file to be deleted")
	}
	entry, found, err = store.Read(ctx, "notes/b.txt")
	if err != nil || !found {
		t.Fatalf("read moved file: found=%v err=%v", found, err)
	}
	if entry.Content != "hi\nearth\n" {
		t.Fatalf("unexpected moved content: %q", entry.Content)
	}

	deletePatch := strings.Join([]string{
		"*** Begin Patch",
		"*** Delete File: notes/b.txt",
		"*** End Patch",
	}, "\n")

	if _, err := ApplyPatch(store, deletePatch); err != nil {
		t.Fatalf("apply delete patch: %v", err)
	}
	_, found, err = store.Read(ctx, "notes/b.txt")
	if err != nil {
		t.Fatalf("read after delete: %v", err)
	}
	if found {
		t.Fatalf("expected file to be deleted")
	}
}

