package meta

import (
	"fmt"
	"regexp"
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

const (
	defaultPaging       = "Slide %d / %d"
	defaultAlign        = "top-left"
	defaultHeadingColor = "#a6da95"
)

var (
	hexColorRe = regexp.MustCompile(`^#?([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	// pagingVerbRe matches a format verb, so the count can be validated.
	pagingVerbRe = regexp.MustCompile(`%[^%]`)
)

// Merge overlays override onto base, field by field. A non-empty field in
// override wins. This is how the precedence chain
// (defaults < user config < frontmatter) is applied.
func Merge(base, override Meta) Meta {
	out := base
	if override.Author != "" {
		out.Author = override.Author
	}
	if override.Date != "" {
		out.Date = override.Date
	}
	if override.Paging != "" {
		out.Paging = override.Paging
	}
	if override.Theme != "" {
		out.Theme = override.Theme
	}
	if override.Align != "" {
		out.Align = override.Align
	}
	if override.HeadingColor != "" {
		out.HeadingColor = override.HeadingColor
	}
	return out
}

// Defaults fills in zero-value fields and replaces values that would render as
// garbage. Call it once, after all configuration sources have been merged.
func (m *Meta) Defaults() { m.defaults() }

// defaults fills in zero-value fields with their defaults and replaces values
// that would render as garbage with the default.
func (m *Meta) defaults() {
	if m.Paging == "" || !validPaging(m.Paging) {
		m.Paging = defaultPaging
	}
	// Anything unrecognized becomes the default rather than silently falling
	// through to centered layout.
	if m.Align != defaultAlign && m.Align != "center" {
		m.Align = defaultAlign
	}
	if !hexColorRe.MatchString(m.HeadingColor) {
		m.HeadingColor = defaultHeadingColor
	}
	m.Author = stripControl(m.Author)
	m.Date = stripControl(m.Date)
	m.Paging = stripControl(m.Paging)
}

// validPaging reports whether the paging format is safe to pass to Sprintf
// with two int arguments. Without this a value like "%s" renders as
// "%!s(int=1)" on every frame.
func validPaging(format string) bool {
	verbs := pagingVerbRe.FindAllString(format, -1)
	if len(verbs) != 2 {
		return false
	}
	for _, v := range verbs {
		if v != "%d" {
			return false
		}
	}
	return true
}

// stripControl removes ANSI escapes and other control characters so that
// frontmatter from an untrusted presentation cannot drive the terminal.
func stripControl(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r == '\t' {
			b.WriteByte(' ')
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Extract parses YAML frontmatter from the beginning of content.
// Returns the metadata, the remaining content with frontmatter removed, and a
// non-nil error if a frontmatter block was present but could not be parsed.
//
// The returned Meta holds only what the frontmatter actually set; defaults are
// deliberately not applied here, so a user config can still supply values that
// the presentation left unset. Call Defaults after merging.
func Extract(content string) (Meta, string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")

	noFrontmatter := func() (Meta, string, error) {
		return Meta{}, content, nil
	}

	if !strings.HasPrefix(content, "---\n") {
		return noFrontmatter()
	}

	rest := content[4:]

	// An empty block ("---\n---\n") is valid frontmatter, but contains no
	// newline between the delimiters for the searches below to find.
	if rest == "---" || strings.HasPrefix(rest, "---\n") {
		return Meta{}, strings.TrimPrefix(strings.TrimPrefix(rest, "---"), "\n"), nil
	}

	// Find closing ---
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		idx = strings.Index(rest, "\n---")
		if idx < 0 || idx+4 != len(rest) {
			return noFrontmatter()
		}
	}

	block := rest[:idx]

	// Require the block to actually be a YAML mapping. Without this check a
	// presentation whose first slide happens to start with a "---" separator
	// has that entire slide consumed as frontmatter and silently dropped.
	var probe map[string]any
	if err := yaml.Unmarshal([]byte(block), &probe); err != nil {
		// The block looks like frontmatter but is not valid YAML. Keep the
		// content rather than discarding it, but report the problem so a
		// typo is not silently ignored.
		m, content, _ := noFrontmatter()
		return m, content, fmt.Errorf("parsing frontmatter: %w", err)
	}
	if len(probe) == 0 {
		// Not a mapping, so not frontmatter — most likely a slide separator.
		return noFrontmatter()
	}

	remaining := rest[idx+4:]
	remaining = strings.TrimPrefix(remaining, "\n")

	var m Meta
	if err := yaml.Unmarshal([]byte(block), &m); err != nil {
		return Meta{}, remaining, fmt.Errorf("parsing frontmatter: %w", err)
	}
	m.Date = formatDate(m.Date)

	return m, remaining, nil
}

// dateCodes are the supported date format codes, longest first so that e.g.
// "MMMM" wins over "MM".
var dateCodes = []struct{ code, layout string }{
	{"YYYY", "2006"},
	{"MMMM", "January"},
	{"MMM", "Jan"},
	{"YY", "06"},
	{"MM", "01"},
	{"dd", "02"},
	{"d", "2"},
}

// formatDate expands date format codes against the current date. Codes are
// only expanded when they stand alone rather than appearing inside a word, so
// a format like "Updated YYYY" keeps its literal text intact.
func formatDate(format string) string {
	if format == "" {
		return ""
	}
	now := time.Now()

	isLetter := func(b byte) bool {
		return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
	}

	var b strings.Builder
	for i := 0; i < len(format); {
		matched := false
		for _, dc := range dateCodes {
			if !strings.HasPrefix(format[i:], dc.code) {
				continue
			}
			// Reject a match that is part of a surrounding word.
			if i > 0 && isLetter(format[i-1]) {
				continue
			}
			if end := i + len(dc.code); end < len(format) && isLetter(format[end]) {
				continue
			}
			b.WriteString(now.Format(dc.layout))
			i += len(dc.code)
			matched = true
			break
		}
		if !matched {
			b.WriteByte(format[i])
			i++
		}
	}
	return b.String()
}
