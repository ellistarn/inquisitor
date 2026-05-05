package inquisitor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePatterns(t *testing.T) {
	// Create temp directories simulating two modules
	mod1 := t.TempDir()
	os.WriteFile(filepath.Join(mod1, "go.mod"), []byte("module example.com/mod1\n\ngo 1.26\n"), 0644)
	os.MkdirAll(filepath.Join(mod1, "pkg", "sub"), 0755)
	os.WriteFile(filepath.Join(mod1, "pkg", "sub", "main.go"), []byte("package sub\n"), 0644)

	mod2 := t.TempDir()
	os.WriteFile(filepath.Join(mod2, "go.mod"), []byte("module example.com/mod2\n\ngo 1.26\n"), 0644)

	tests := []struct {
		name     string
		patterns []string
		wantRoot string // if non-empty, assert root matches
		wantPats []string
		wantErr  string
	}{
		{
			name:     "import path passes through",
			patterns: []string{"github.com/foo/bar/..."},
			wantRoot: "",
			wantPats: []string{"github.com/foo/bar/..."},
		},
		{
			name:     "relative ./... resolves as filesystem path",
			patterns: []string{"./..."},
		},
		{
			name:     "absolute path to subdirectory",
			patterns: []string{filepath.Join(mod1, "pkg")},
			wantRoot: mod1,
			wantPats: []string{"./pkg/..."},
		},
		{
			name:     "absolute path with /... suffix",
			patterns: []string{filepath.Join(mod1, "pkg") + "/..."},
			wantRoot: mod1,
			wantPats: []string{"./pkg/..."},
		},
		{
			name:     "module root directory",
			patterns: []string{mod1},
			wantRoot: mod1,
			wantPats: []string{"./..."},
		},
		{
			name:     "multiple patterns same module",
			patterns: []string{filepath.Join(mod1, "pkg"), mod1},
			wantRoot: mod1,
			wantPats: []string{"./pkg/...", "./..."},
		},
		{
			name:     "patterns span multiple modules",
			patterns: []string{mod1, mod2},
			wantErr:  "patterns span multiple modules",
		},
		{
			name:     "mixed filesystem and import path",
			patterns: []string{mod1, "github.com/foo/bar"},
			wantErr:  "cannot mix filesystem paths with import path patterns",
		},
		{
			name:     "nonexistent path",
			patterns: []string{"/nonexistent/path/to/nothing"},
			wantErr:  "stat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pats, root, err := resolvePatterns(tt.patterns)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantRoot != "" && root != tt.wantRoot {
				t.Errorf("root = %q, want %q", root, tt.wantRoot)
			}
			if tt.wantPats != nil && !sliceEqual(pats, tt.wantPats) {
				t.Errorf("patterns = %v, want %v", pats, tt.wantPats)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
