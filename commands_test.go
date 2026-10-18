package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "new.go")
	if err := createFile(p); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(p); err != nil || fi.Size() != 0 {
		t.Fatalf("want an empty file, got %v, %v", fi, err)
	}
	if err := createFile(p); err == nil {
		t.Error("an existing file must not be overwritten")
	}
	if err := createFile(filepath.Join(dir, "missing", "x.go")); err == nil {
		t.Error("missing directories must not be created")
	}
	if _, err := os.Stat(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Error("directory was created")
	}
}
