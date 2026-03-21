package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyAndRemove(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "source.txt")
	dstPath := filepath.Join(dstDir, "dest.txt")

	content := []byte("hello cross-device")
	if err := os.WriteFile(srcPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyAndRemove(srcPath, dstPath); err != nil {
		t.Fatalf("copyAndRemove: %v", err)
	}

	// Source must be removed.
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("source file should have been removed")
	}

	// Destination must contain the original content.
	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}

func TestMoveFileSameDevice(t *testing.T) {
	dir := t.TempDir()
	tmpDir, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tmpDir.Close()

	content := []byte("same device move")
	if err := os.WriteFile(filepath.Join(dir, "item.png"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	dstDir := t.TempDir()
	dstPath := filepath.Join(dstDir, "sub", "item.png")

	if err := moveFile(tmpDir, "item.png", dstPath); err != nil {
		t.Fatalf("moveFile: %v", err)
	}

	// Source must be gone (renamed).
	if _, err := os.Stat(filepath.Join(dir, "item.png")); !os.IsNotExist(err) {
		t.Error("source should have been moved")
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}
