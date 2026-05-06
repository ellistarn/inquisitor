package inquisitor

import (
	"os"
	"path/filepath"
	"strings"
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
			assertError(t, err, tt.wantErr)
			if tt.wantErr != "" {
				return
			}
			assertStringEqual(t, "root", root, tt.wantRoot)
			assertSliceEqual(t, "patterns", pats, tt.wantPats)
		})
	}
}

func assertError(t *testing.T, err error, wantErr string) {
	t.Helper()
	if wantErr == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("expected error containing %q, got %q", wantErr, err.Error())
	}
}

func assertStringEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if want == "" {
		return
	}
	if got != want {
		t.Errorf("%s = %q, want %q", field, got, want)
	}
}

func assertSliceEqual(t *testing.T, field string, got, want []string) {
	t.Helper()
	if want == nil {
		return
	}
	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v", field, got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %q, want %q", field, i, got[i], want[i])
		}
	}
}
