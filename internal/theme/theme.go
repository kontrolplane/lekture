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
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// Load reads a glamour-compatible JSON theme from a local path or HTTP URL.
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

func loadBytes(path, baseDir string) ([]byte, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		resp, err := httpClient.Get(path)
		if err != nil {
			return nil, fmt.Errorf("fetching theme: %w", err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading theme response: %w", err)
		}
		return data, nil
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	path = filepath.Clean(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading theme file: %w", err)
	}
	return data, nil
}
