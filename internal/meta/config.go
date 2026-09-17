package meta

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFileName is the user configuration file, relative to the config dir.
const ConfigFileName = "lekture/config.yaml"

// ConfigPath returns the path of the user configuration file. It follows the
// XDG convention on every platform, including macOS: lekture's users are
// terminal users, and its peers (git, gh, glow) all keep their config under
// ~/.config rather than ~/Library/Application Support.
func ConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, ConfigFileName)
}

// LoadUserConfig reads the user configuration file. A missing file is not an
// error: it returns a zero Meta so the caller can merge it unconditionally.
//
// A relative theme path is resolved against the config file's own directory,
// not the presentation's, so a house theme kept next to the config works from
// any deck.
func LoadUserConfig() (Meta, error) {
	path := ConfigPath()
	if path == "" {
		return Meta{}, nil
	}
	return loadConfigFile(path)
}

func loadConfigFile(path string) (Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Meta{}, nil
		}
		return Meta{}, fmt.Errorf("reading config %s: %w", path, err)
	}

	var m Meta
	dec := yaml.NewDecoder(bytes.NewReader(data))
	// Reject unknown keys rather than silently ignoring a typo.
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil && !errors.Is(err, fs.ErrNotExist) {
		// An empty file decodes to io.EOF, which is not a problem.
		if err.Error() != "EOF" {
			return Meta{}, fmt.Errorf("parsing config %s: %w", path, err)
		}
	}

	m.Date = formatDate(m.Date)
	m.Theme = resolveConfigTheme(m.Theme, filepath.Dir(path))
	return m, nil
}

// resolveConfigTheme makes a relative theme path absolute against the config
// directory. Built-in names and urls are left alone.
func resolveConfigTheme(theme, configDir string) string {
	switch {
	case theme == "",
		filepath.IsAbs(theme),
		strings.HasPrefix(theme, "http://"),
		strings.HasPrefix(theme, "https://"),
		!strings.ContainsAny(theme, `/\.`):
		return theme
	}
	return filepath.Join(configDir, theme)
}
