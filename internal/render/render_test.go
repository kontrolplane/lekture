package render

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/kontrolplane/lekture/internal/parser"
)

// TestExtractLeadingWhitespaceTruncatedCSI guards an operator-precedence bug
// that let the bounds check be skipped, panicking on any string ending inside
// an ANSI escape sequence.
func TestExtractLeadingWhitespaceTruncatedCSI(t *testing.T) {
	cases := []struct{ in, want string }{
		{"    ", "    "},
		{"\x1b[0m    ", "    "},
		{"  \x1b[38;2;1;2;3m  x", "    "},
		{"\x1b[0m", ""},
		{"\x1b[1mabc", ""},
		{"\t\tx", "\t\t"},
		{"\x1b[", ""},
		{"  \x1b[38;5;1", "  "},
	}
	for _, tc := range cases {
		got := extractLeadingWhitespace(tc.in)
		if got != tc.want {
			t.Errorf("extractLeadingWhitespace(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseHex(t *testing.T) {
	cases := []struct {
		in      string
		r, g, b uint8
	}{
		{"#a6da95", 166, 218, 149},
		{"a6da95", 166, 218, 149},
		{"#fff", 255, 255, 255},
		{"#ABCDEF", 171, 205, 239},
		{"#00ff00", 0, 255, 0},
		{"", 136, 136, 136},
		{"#12345", 136, 136, 136},
		{"#zzzzzz", 136, 136, 136},
		{"#8g8g8g", 136, 136, 136},
	}
	for _, tc := range cases {
		r, g, b := parseHex(tc.in)
		if r != tc.r || g != tc.g || b != tc.b {
			t.Errorf("parseHex(%q) = (%d,%d,%d), want (%d,%d,%d)", tc.in, r, g, b, tc.r, tc.g, tc.b)
		}
	}
}

func TestBlendHex(t *testing.T) {
	cases := []struct {
		a, b string
		tt   float64
		want string
	}{
		{"#000000", "#ffffff", 0.5, "#7f7f7f"},
		{"#000000", "#888888", 0, "#000000"},
		{"#000000", "#888888", 1, "#888888"},
		{"bogus", "#888888", 0.5, "#888888"},
	}
	for _, tc := range cases {
		if got := blendHex(tc.a, tc.b, tc.tt); got != tc.want {
			t.Errorf("blendHex(%q,%q,%v) = %q, want %q", tc.a, tc.b, tc.tt, got, tc.want)
		}
	}
}

// TestContrastColor checks that heading text stays readable regardless of the
// configured heading color.
func TestContrastColor(t *testing.T) {
	if got := contrastColor("#a6da95"); got != "#1e1e2e" {
		t.Errorf("light background got %q, want the dark foreground", got)
	}
	if got := contrastColor("#14325C"); got != "#f5f5f5" {
		t.Errorf("dark background got %q, want the light foreground", got)
	}
}

// TestHardWrapPreventsOverflow guards against a single unbreakable token (a
// long URL or identifier) running past the terminal width, which wraps at the
// terminal level and corrupts the full-screen layout.
func TestHardWrapPreventsOverflow(t *testing.T) {
	md := "# Title\n\n" +
		"See https://example.com/some/really/long/path/that/goes/on/and/on/and/on/forever/and/ever/nicely\n\n" +
		"```go\nveryLongVariableNameThatGoesOnForeverAndEverWithoutAnySpacesOrBreaksAtAllInThisLine := 1\n```\n"

	for _, width := range []int{40, 60, 80, 120} {
		r := New(t.TempDir(), width, 40, "", "#a6da95")
		out, err := r.RenderSlide(parser.Slide{RawMarkdown: md})
		if err != nil {
			t.Fatalf("width %d: %v", width, err)
		}
		contentWidth := width * 3 / 4
		if contentWidth < 20 {
			contentWidth = 20
		}
		for i, line := range strings.Split(out, "\n") {
			if w := xansi.StringWidth(line); w > contentWidth {
				t.Errorf("width %d: line %d is %d columns, want <= %d: %q",
					width, i, w, contentWidth, xansi.Strip(line))
			}
		}
	}
}

func TestHardWrapKeepsContent(t *testing.T) {
	long := strings.Repeat("abcdefghij", 12) // 120 chars, unbreakable
	got := xansi.Strip(hardWrap(long, 30))
	joined := strings.ReplaceAll(got, "\n", "")
	if joined != long {
		t.Errorf("hard wrap lost or altered content:\ngot  %q\nwant %q", joined, long)
	}
	for _, line := range strings.Split(got, "\n") {
		if len(line) > 30 {
			t.Errorf("line too long: %q", line)
		}
	}
}
