package theme

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/kontrolplane/lekture/themes"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// maxThemeBytes caps how much is read from a remote theme, so a misbehaving
// server cannot exhaust memory.
const maxThemeBytes = 1 << 20 // 1 MiB

// Load reads a glamour-compatible JSON theme. The path may be the name of a
// bundled theme, a local file path, or an http(s) URL.
func Load(path string, baseDir string) (*ansi.StyleConfig, error) {
	data, err := loadBytes(path, baseDir)
	if err != nil {
		return nil, err
	}

	var style ansi.StyleConfig
	if err := json.Unmarshal(data, &style); err != nil {
		return nil, fmt.Errorf("parsing theme JSON: %w", err)
	}
	return &style, nil
}

// BuiltinNames lists the bundled theme names.
func BuiltinNames() []string { return themes.Names() }

func loadBytes(path, baseDir string) ([]byte, error) {
	// A bare name refers to a bundled theme.
	if data, ok := themes.Lookup(path); ok {
		return data, nil
	}

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return fetch(path)
	}

	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(baseDir, resolved)
	}
	resolved = filepath.Clean(resolved)

	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("reading theme file: %w", err)
	}
	return data, nil
}

func fetch(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching theme: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching theme: unexpected status %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxThemeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading theme response: %w", err)
	}
	if len(data) > maxThemeBytes {
		return nil, fmt.Errorf("theme exceeds %d bytes", maxThemeBytes)
	}
	return data, nil
}
