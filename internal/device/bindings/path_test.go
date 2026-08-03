package bindings

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateAndNormalizePathDir(t *testing.T) {
	dir := t.TempDir()
	got, err := ValidateAndNormalizePath(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(got)
	if err != nil || !info.IsDir() {
		t.Fatalf("normalized path not a dir: %s %v", got, err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("expected absolute, got %s", got)
	}
}

func TestValidateRejectsMissingAndFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope")
	if _, err := ValidateAndNormalizePath(missing); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("missing: want ErrInvalidPath, got %v", err)
	}

	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateAndNormalizePath(file); !errors.Is(err, ErrNotDir) {
		t.Fatalf("file: want ErrNotDir, got %v", err)
	}
}

func TestValidateRelativeResolves(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ValidateAndNormalizePath("sub")
	if err != nil {
		t.Fatal(err)
	}
	if !PathsEqual(got, sub) && !PathsEqual(got, filepath.Clean(sub)) {
		// Compare via EvalSymlinks of expected too
		want, _ := filepath.EvalSymlinks(sub)
		if !PathsEqual(got, want) {
			t.Fatalf("got %s want ~%s", got, sub)
		}
	}
}

func TestPathsEqualWindowsCase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only case fold")
	}
	if !PathsEqual(`C:\Data\Movies`, `c:\data\movies`) {
		t.Fatal("expected case-insensitive equality")
	}
}

func TestPutValidatedConflict(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	bdir := filepath.Join(root, "b")
	_ = os.Mkdir(a, 0o700)
	_ = os.Mkdir(bdir, 0o700)

	store := Open(filepath.Join(root, "bindings.json"))
	if err := store.PutValidated(Binding{FolderID: "fld_a", LocalPath: a, Subscribed: true}); err != nil {
		t.Fatal(err)
	}
	err := store.PutValidated(Binding{FolderID: "fld_b", LocalPath: a, Subscribed: true})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}
