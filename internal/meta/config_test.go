package meta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPathFollowsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	want := filepath.Join("/custom/config", ConfigFileName)
	if got := ConfigPath(); got != want {
		t.Errorf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestConfigPathFallsBackToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	os.Unsetenv("XDG_CONFIG_HOME")
	got := ConfigPath()
	if !strings.HasSuffix(got, filepath.Join(".config", ConfigFileName)) {
		t.Errorf("ConfigPath() = %q, want it under ~/.config", got)
	}
}

// TestMissingConfigIsNotAnError matters because most users will never create
// one: a missing file must never interrupt a presentation.
func TestMissingConfigIsNotAnError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("err = %v, want nil for a missing config", err)
	}
	if m != (Meta{}) {
		t.Errorf("expected a zero Meta, got %+v", m)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, ConfigFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadUserConfigReadsFields(t *testing.T) {
	writeConfig(t, "author: levi\nalign: center\nheadingColor: \"#F4E8C1\"\npaging: \"%d of %d\"\n")
	m, err := LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if m.Author != "levi" || m.Align != "center" || m.HeadingColor != "#F4E8C1" || m.Paging != "%d of %d" {
		t.Errorf("config not read correctly: %+v", m)
	}
}

func TestEmptyConfigIsFine(t *testing.T) {
	writeConfig(t, "")
	if _, err := LoadUserConfig(); err != nil {
		t.Errorf("err = %v, want nil for an empty config", err)
	}
}

// TestUnknownKeyIsRejected keeps a typo from being silently ignored.
func TestUnknownKeyIsRejected(t *testing.T) {
	writeConfig(t, "authr: levi\n")
	if _, err := LoadUserConfig(); err == nil {
		t.Error("expected an error for an unknown key")
	}
}

func TestMalformedConfigIsReported(t *testing.T) {
	writeConfig(t, "author: [unclosed\n")
	if _, err := LoadUserConfig(); err == nil {
		t.Error("expected an error for malformed yaml")
	}
}

// TestConfigThemePathResolvesAgainstConfigDir means a house theme kept beside
// the config works from any presentation directory.
func TestConfigThemePathResolvesAgainstConfigDir(t *testing.T) {
	path := writeConfig(t, "theme: mytheme.json\n")
	m, err := LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(path), "mytheme.json")
	if m.Theme != want {
		t.Errorf("theme = %q, want %q", m.Theme, want)
	}
}

func TestConfigThemeBuiltinAndURLLeftAlone(t *testing.T) {
	for _, theme := range []string{"kontrolplane", "https://example.com/t.json"} {
		writeConfig(t, "theme: "+theme+"\n")
		m, err := LoadUserConfig()
		if err != nil {
			t.Fatal(err)
		}
		if m.Theme != theme {
			t.Errorf("theme %q was rewritten to %q", theme, m.Theme)
		}
	}
}

func TestConfigDateIsFormatted(t *testing.T) {
	writeConfig(t, "date: YYYY\n")
	m, err := LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if m.Date == "YYYY" || len(m.Date) != 4 {
		t.Errorf("date = %q, want the year", m.Date)
	}
}
