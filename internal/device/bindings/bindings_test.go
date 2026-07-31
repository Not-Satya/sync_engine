package bindings

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestPutGetListRemove(t *testing.T) {
	dir := t.TempDir()
	store := Open(filepath.Join(dir, "folder_bindings.json"))

	list, err := store.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("empty list: %v %#v", err, list)
	}

	b := Binding{
		FolderID:   "fld_movies",
		LocalPath:  `D:\Movies`,
		Name:       "Movies",
		Subscribed: true,
		BoundAt:    time.Now().UTC(),
	}
	if err := store.Put(b); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get("fld_movies")
	if err != nil {
		t.Fatal(err)
	}
	if got.LocalPath != b.LocalPath || !got.Subscribed || got.Name != "Movies" {
		t.Fatalf("get: %+v", got)
	}

	byPath, err := store.GetByPath(`D:\Movies`)
	if err != nil || byPath.FolderID != "fld_movies" {
		t.Fatalf("by path: %+v %v", byPath, err)
	}

	list, err = store.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %#v", err, list)
	}

	if err := store.SetSubscribed("fld_movies", false); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get("fld_movies")
	if got.Subscribed {
		t.Fatal("expected unsubscribed")
	}

	if err := store.Remove("fld_movies"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("fld_movies"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestPutPathConflict(t *testing.T) {
	dir := t.TempDir()
	store := Open(filepath.Join(dir, "folder_bindings.json"))

	_ = store.Put(Binding{FolderID: "fld_a", LocalPath: `/data/a`, Subscribed: true})
	err := store.Put(Binding{FolderID: "fld_b", LocalPath: `/data/a`, Subscribed: true})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestPutReplaceSameFolder(t *testing.T) {
	dir := t.TempDir()
	store := Open(filepath.Join(dir, "folder_bindings.json"))

	_ = store.Put(Binding{FolderID: "fld_a", LocalPath: `/old`, Name: "A", Subscribed: true})
	if err := store.Put(Binding{FolderID: "fld_a", LocalPath: `/new`, Name: "A2", Subscribed: false}); err != nil {
		t.Fatal(err)
	}
	list, _ := store.List()
	if len(list) != 1 || list[0].LocalPath != `/new` || list[0].Name != "A2" {
		t.Fatalf("replace: %+v", list)
	}
}
