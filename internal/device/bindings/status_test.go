package bindings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPathAndStatus(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "movies")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if h, _ := CheckPath(dir); h != PathOK {
		t.Fatalf("dir health=%s", h)
	}
	if h, _ := CheckPath(filepath.Join(root, "missing")); h != PathMissing {
		t.Fatalf("missing health=%s", h)
	}
	if h, _ := CheckPath(file); h != PathNotDir {
		t.Fatalf("file health=%s", h)
	}

	store := Open(filepath.Join(root, "bindings.json"))
	_ = store.Put(Binding{FolderID: "fld_1", LocalPath: dir, Name: "Movies", Subscribed: true})
	_ = store.Put(Binding{FolderID: "fld_2", LocalPath: file, Name: "Bad", Subscribed: false})

	rows, err := store.Status()
	if err != nil || len(rows) != 2 {
		t.Fatalf("status: %v %#v", err, rows)
	}
	byID := map[string]StatusRow{}
	for _, r := range rows {
		byID[r.Binding.FolderID] = r
	}
	if byID["fld_1"].Health != PathOK || byID["fld_2"].Health != PathNotDir {
		t.Fatalf("rows: %+v", rows)
	}
}
