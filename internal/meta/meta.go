package meta

import (
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Meta holds presentation metadata parsed from YAML frontmatter.
type Meta struct {
	Author       string `yaml:"author" json:"author"`
	Date         string `yaml:"date" json:"date"`
	Paging       string `yaml:"paging" json:"paging"`
	Theme        string `yaml:"theme" json:"theme"`
	Align        string `yaml:"align" json:"align"`               // "top-left" (default) or "center"
	HeadingColor string `yaml:"headingColor" json:"headingColor"` // hex color for headings, e.g. "#a6da95"
}

// defaults fills in zero-value fields with their defaults.
func (m *Meta) defaults() {
	if m.Paging == "" {
		m.Paging = "Slide %d / %d"
	}
	if m.Align == "" {
		m.Align = "top-left"
	}
	if m.HeadingColor == "" {
		m.HeadingColor = "#a6da95"
	}
}

// Extract parses YAML frontmatter from the beginning of content.
// Returns the metadata and the remaining content with frontmatter removed.
func Extract(content string) (Meta, string) {
	content = strings.ReplaceAll(content, "\r\n", "\n")

	if !strings.HasPrefix(content, "---\n") {
		var m Meta
		m.defaults()
		return m, content
	}

	// Find closing ---
	rest := content[4:]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		idx = strings.Index(rest, "\n---")
		if idx < 0 || idx+4 != len(rest) {
			var m Meta
			m.defaults()
			return m, content
		}
	}

	var m Meta
	_ = yaml.Unmarshal([]byte(rest[:idx]), &m)
	m.Date = formatDate(m.Date)
	m.defaults()

	remaining := rest[idx+4:]
	if len(remaining) > 0 && remaining[0] == '\n' {
		remaining = remaining[1:]
	}
	return m, remaining
}

func formatDate(format string) string {
	if format == "" {
		return ""
	}
	now := time.Now()
	// Match slides date format codes: YYYY, YY, MMMM, MMM, MM, dd, d
	r := strings.NewReplacer(
		"YYYY", now.Format("2006"),
		"YY", now.Format("06"),
		"MMMM", now.Format("January"),
		"MMM", now.Format("Jan"),
		"MM", now.Format("01"),
		"dd", now.Format("02"),
		"d", now.Format("2"),
	)
	return r.Replace(format)
}
