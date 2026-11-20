package window

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFontSizeScalesInverselyWithColumns(t *testing.T) {
	cases := []struct {
		got, want int
		size      float64
	}{
		{122, 100, 24.4}, // fewer columns wanted: bigger font
		{122, 122, 20.0},
		{122, 244, 10.0}, // more columns wanted: smaller font
		{100, 92, 21.7},
	}
	for _, c := range cases {
		if got := FontSize(20, c.got, c.want); got != c.size {
			t.Errorf("FontSize(20, %d, %d) = %v, want %v", c.got, c.want, got, c.size)
		}
	}
}

func TestFontSizeIsClamped(t *testing.T) {
	if got := FontSize(20, 400, 10); got != maxFont {
		t.Errorf("huge font = %v, want the %v cap", got, maxFont)
	}
	if got := FontSize(20, 10, 400); got != minFont {
		t.Errorf("tiny font = %v, want the %v floor", got, minFont)
	}
	if got := FontSize(20, 0, 100); got != 20 {
		t.Errorf("a failed measurement should leave the probe size, got %v", got)
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"plain":         `'plain'`,
		"with space":    `'with space'`,
		"it's":          `'it'\''s'`,
		"$(rm -rf) `x`": "'$(rm -rf) `x`'",
		"":              `''`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestParseSize(t *testing.T) {
	rows, cols, err := parseSize("34 122\n")
	if err != nil || rows != 34 || cols != 122 {
		t.Fatalf("parseSize = %d, %d, %v", rows, cols, err)
	}
	for _, bad := range []string{"", "34", "a b", "0 0", "1 2 3"} {
		if _, _, err := parseSize(bad); err == nil {
			t.Errorf("parseSize(%q) should fail", bad)
		}
	}
}

func TestOpenRejectsSillyColumns(t *testing.T) {
	for _, n := range []int{0, MinColumns - 1, MaxColumns + 1} {
		err := Open("lekture", ".", "deck.md", n)
		if err == nil || !strings.Contains(err.Error(), "--columns") {
			t.Errorf("Open with %d columns: err = %v", n, err)
		}
	}
}

func TestGhosttyArgsCarryFontAndCommand(t *testing.T) {
	args := ghosttyArgs(24.4, "'lekture' 'deck.md'")
	joined := strings.Join(args, "\n")
	for _, want := range []string{"--font-size=24.4", "--command='lekture' 'deck.md'", "--fullscreen=true", "--window-save-state=never"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q:\n%s", want, joined)
		}
	}
}

func TestWriteSizeWritesRowsThenColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "size")
	if err := WriteSize(path, func() (int, int, error) { return 122, 34, nil }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, cols, err := parseSize(string(data))
	if err != nil || rows != 34 || cols != 122 {
		t.Fatalf("round trip = %d rows, %d cols, %v", rows, cols, err)
	}
}
