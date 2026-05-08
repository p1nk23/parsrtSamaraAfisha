package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type testItem struct {
	Name string `json:"name"`
}

func TestFileStoreSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "events.json")
	store := FileStore[[]testItem]{Path: path}

	want := []testItem{{Name: "event"}}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "event" {
		t.Fatalf("unexpected loaded value: %#v", got)
	}
}

func TestFileStoreLoadMissing(t *testing.T) {
	store := FileStore[[]testItem]{Path: filepath.Join(t.TempDir(), "missing.json")}
	_, err := store.Load()
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}
