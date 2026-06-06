package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFrontendDirFromNestedCommandDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module exchange\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	frontendDir := filepath.Join(root, "frontend")
	if err := os.Mkdir(frontendDir, 0o755); err != nil {
		t.Fatal(err)
	}
	nestedDir := filepath.Join(root, "cmd", "executor")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.Chdir(nestedDir); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveFrontendDir("frontend")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(frontendDir)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("resolved frontend dir = %q, want %q", actual, expected)
	}
}
