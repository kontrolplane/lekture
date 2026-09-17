package meta

import (
	"strings"
	"testing"
	"time"
)

func TestExtract(t *testing.T) {
	// Extract returns only what the frontmatter set; defaults are applied
	// later, after the user config has been merged underneath.
	cases := []struct {
		name, in   string
		wantAuthor string
		wantAlign  string
		wantRest   string
		wantErr    bool
	}{
		{"basic", "---\nauthor: Levi\n---\n# Slide", "Levi", "", "# Slide", false},
		{"crlf", "---\r\nauthor: Levi\r\n---\r\n# Slide", "Levi", "", "# Slide", false},
		{"no frontmatter", "# Slide\n\n---\n\n# Two", "", "", "# Slide\n\n---\n\n# Two", false},
		{"align center", "---\nalign: center\n---\nx", "", "center", "x", false},
		{"unterminated", "---\nauthor: Levi\n# Slide", "", "", "---\nauthor: Levi\n# Slide", false},
		{"eof close", "---\nauthor: Levi\n---", "Levi", "", "", false},
		// An empty frontmatter block is valid and must be stripped.
		{"empty block", "---\n---\n# Slide", "", "", "# Slide", false},
		// A deck whose first slide opens with a separator must not have that
		// slide consumed as frontmatter.
		{"leading separator", "---\n\n# Slide 1\n\n---\n\n# Slide 2", "", "", "---\n\n# Slide 1\n\n---\n\n# Slide 2", false},
		{"comment only is not frontmatter", "---\n# Heading\n---\nSlide 2", "", "", "---\n# Heading\n---\nSlide 2", false},
		// Malformed frontmatter is reported, but its content is preserved
		// rather than silently discarded.
		{"malformed yaml reports error", "---\nauthor: [unclosed\n---\n# Slide", "", "", "---\nauthor: [unclosed\n---\n# Slide", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, rest, err := Extract(tc.in)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if m.Author != tc.wantAuthor {
				t.Errorf("author = %q, want %q", m.Author, tc.wantAuthor)
			}
			if m.Align != tc.wantAlign {
				t.Errorf("align = %q, want %q", m.Align, tc.wantAlign)
			}
			if rest != tc.wantRest {
				t.Errorf("rest = %q, want %q", rest, tc.wantRest)
			}
		})
	}
}

func TestDefaultsAppliedAfterExtract(t *testing.T) {
	m, _, err := Extract("---\nauthor: Levi\n---\n# Slide")
	if err != nil {
		t.Fatal(err)
	}
	m.Defaults()
	if m.Align != defaultAlign {
		t.Errorf("align = %q, want %q", m.Align, defaultAlign)
	}
	if m.Paging != defaultPaging {
		t.Errorf("paging = %q, want %q", m.Paging, defaultPaging)
	}
	if m.HeadingColor != defaultHeadingColor {
		t.Errorf("headingColor = %q, want %q", m.HeadingColor, defaultHeadingColor)
	}
}

func TestAlignInvalidFallsBack(t *testing.T) {
	m := Meta{Align: "sideways"}
	m.Defaults()
	if m.Align != defaultAlign {
		t.Errorf("align = %q, want %q", m.Align, defaultAlign)
	}
}

func TestFormatDateLeavesProseAlone(t *testing.T) {
	now := time.Now()
	cases := []struct{ in, want string }{
		{"", ""},
		{"YYYY-MM-dd", now.Format("2006-01-02")},
		{"dd/MM/YY", now.Format("02/01/06")},
		{"MMMM d, YYYY", now.Format("January") + " " + now.Format("2") + ", " + now.Format("2006")},
		{"d MMM YYYY", now.Format("2") + " " + now.Format("Jan") + " " + now.Format("2006")},
		// A bare "d" inside a word must not be expanded.
		{"Updated: YYYY", "Updated: " + now.Format("2006")},
		{"Wednesday", "Wednesday"},
	}
	for _, tc := range cases {
		if got := formatDate(tc.in); got != tc.want {
			t.Errorf("formatDate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPagingValidation(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", defaultPaging},
		{"%d / %d", "%d / %d"},
		{"Slide %d of %d", "Slide %d of %d"},
		{"%s", defaultPaging},       // wrong verb
		{"%d", defaultPaging},       // too few
		{"%d %d %d", defaultPaging}, // too many
	}
	for _, tc := range cases {
		m := Meta{Paging: tc.in}
		m.defaults()
		if m.Paging != tc.want {
			t.Errorf("paging %q -> %q, want %q", tc.in, m.Paging, tc.want)
		}
	}
}

func TestHeadingColorValidation(t *testing.T) {
	for _, in := range []string{"", "notacolor", "#12345", "rgb(1,2,3)"} {
		m := Meta{HeadingColor: in}
		m.defaults()
		if m.HeadingColor != defaultHeadingColor {
			t.Errorf("headingColor %q -> %q, want default", in, m.HeadingColor)
		}
	}
	for _, in := range []string{"#fff", "#a6da95", "a6da95"} {
		m := Meta{HeadingColor: in}
		m.defaults()
		if m.HeadingColor != in {
			t.Errorf("headingColor %q was rejected", in)
		}
	}
}

func TestControlCharactersStripped(t *testing.T) {
	m := Meta{Author: "evil\x1b]0;pwned\x07"}
	m.defaults()
	if strings.ContainsRune(m.Author, '\x1b') {
		t.Errorf("author still contains an escape: %q", m.Author)
	}
}

func TestMergePrecedence(t *testing.T) {
	config := Meta{Author: "config author", Theme: "kontrolplane", Align: "center"}
	frontmatter := Meta{Author: "deck author", HeadingColor: "#ffffff"}

	got := Merge(config, frontmatter)

	// Frontmatter wins where it sets a value.
	if got.Author != "deck author" {
		t.Errorf("author = %q, want the frontmatter value", got.Author)
	}
	// Config fills in what the deck left unset.
	if got.Theme != "kontrolplane" {
		t.Errorf("theme = %q, want the config value", got.Theme)
	}
	if got.Align != "center" {
		t.Errorf("align = %q, want the config value", got.Align)
	}
	// Frontmatter-only fields survive.
	if got.HeadingColor != "#ffffff" {
		t.Errorf("headingColor = %q, want the frontmatter value", got.HeadingColor)
	}
}

// TestConfigIsNotClobberedByDefaults is the ordering bug this design exists to
// avoid: applying defaults inside Extract would fill every field, leaving
// nothing for the config to supply.
func TestConfigIsNotClobberedByDefaults(t *testing.T) {
	config := Meta{Align: "center", Paging: "%d of %d"}
	frontmatter, _, err := Extract("---\nauthor: Levi\n---\n# Slide")
	if err != nil {
		t.Fatal(err)
	}
	merged := Merge(config, frontmatter)
	merged.Defaults()

	if merged.Align != "center" {
		t.Errorf("align = %q, want the config value to survive", merged.Align)
	}
	if merged.Paging != "%d of %d" {
		t.Errorf("paging = %q, want the config value to survive", merged.Paging)
	}
	if merged.Author != "Levi" {
		t.Errorf("author = %q, want the frontmatter value", merged.Author)
	}
}
