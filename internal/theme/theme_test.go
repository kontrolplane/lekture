package theme

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuiltinThemesLoadByName is the fix for bundled themes only resolving
// from inside a repository checkout.
func TestBuiltinThemesLoadByName(t *testing.T) {
	names := BuiltinNames()
	if len(names) == 0 {
		t.Fatal("no bundled themes were embedded")
	}
	for _, name := range names {
		// baseDir is deliberately unrelated: a built-in name must resolve
		// without any file next to the presentation.
		if _, err := Load(name, t.TempDir()); err != nil {
			t.Errorf("Load(%q) = %v, want it to resolve from the embedded themes", name, err)
		}
	}
}

func TestLoadRejectsNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Load(srv.URL, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want a 404 to be reported", err)
	}
}

func TestLoadCapsResponseSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := strings.Repeat("a", 1<<16)
		for i := 0; i < 32; i++ {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	_, err := Load(srv.URL, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("err = %v, want the size cap to be enforced", err)
	}
}

func TestLoadLocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.json")
	if err := os.WriteFile(path, []byte(`{"document":{"color":"#ffffff"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("custom.json", dir); err != nil {
		t.Errorf("Load = %v, want success", err)
	}
}

func TestLoadReportsMissingFile(t *testing.T) {
	if _, err := Load("nope.json", t.TempDir()); err == nil {
		t.Error("expected an error for a missing theme file")
	}
}

func TestBadThemeIsReportedNotSwallowed(t *testing.T) {
	if _, err := Load("does-not-exist", t.TempDir()); err == nil {
		t.Fatal("a missing theme must report an error rather than silently falling back")
	}
}
