package image

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muesli/termenv"
)

func writePNG(t *testing.T, img image.Image) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "img.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSemiTransparentPixelIsNotDarkened guards the premultiplied-alpha bug:
// RGBA() returns premultiplied values, so using them directly rendered
// semi-transparent pixels far darker than their real color.
func TestSemiTransparentPixelIsNotDarkened(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 0, B: 0, A: 200})
	r, g, b, opaque := sample(img, 0, 0)
	if !opaque {
		t.Fatal("pixel should count as visible at alpha 200")
	}
	if r < 250 || g != 0 || b != 0 {
		t.Errorf("sample = (%d,%d,%d), want approximately (255,0,0)", r, g, b)
	}
}

func TestFullyTransparentPixelIsNotDrawn(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{})
	if _, _, _, opaque := sample(img, 0, 0); opaque {
		t.Error("a fully transparent pixel should not be drawn")
	}
}

// TestRenderCoalescesColors checks that a flat image does not re-emit an
// escape sequence for every identical cell.
func TestRenderCoalescesColors(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	out, err := Render(writePNG(t, img), 20, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, "\x1b[38;2;"); n > 12 {
		t.Errorf("emitted %d foreground escapes for a flat image, want them coalesced per line", n)
	}
}

func TestRenderIsDeterministicAndCached(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 30), G: uint8(y * 30), B: 90, A: 255})
		}
	}
	path := writePNG(t, img)
	first, err := Render(path, 8, 4)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(path, 8, 4)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("repeated renders differ")
	}
}

func TestRenderRejectsDirectory(t *testing.T) {
	if _, err := Render(t.TempDir(), 10, 10); err == nil {
		t.Error("expected an error for a directory")
	}
}

func TestRenderMissingFile(t *testing.T) {
	if _, err := Render(filepath.Join(t.TempDir(), "nope.png"), 10, 10); err == nil {
		t.Error("expected an error for a missing file")
	}
}

// TestColorDegradation checks that the renderer emits color appropriate to
// the terminal rather than always writing 24-bit escapes.
func TestColorDegradation(t *testing.T) {
	cases := []struct {
		name     string
		profile  termenv.Profile
		wantTrue bool
		wantAny  bool
	}{
		{"truecolor", termenv.TrueColor, true, true},
		{"256 color", termenv.ANSI256, false, true},
		{"16 color", termenv.ANSI, false, true},
		{"no color", termenv.Ascii, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &sequencer{profile: tc.profile, cache: map[uint32]string{}}
			seq := s.seq(10, 20, 30, false)
			if (seq != "") != tc.wantAny {
				t.Errorf("sequence = %q, wantAny = %v", seq, tc.wantAny)
			}
			if got := strings.Contains(seq, "38;2;"); got != tc.wantTrue {
				t.Errorf("truecolor = %v, want %v (seq %q)", got, tc.wantTrue, seq)
			}
		})
	}
}

func TestResolveProfileHonorsEnvironment(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want termenv.Profile
	}{
		{"NO_COLOR wins", map[string]string{"NO_COLOR": "1", "COLORTERM": "truecolor"}, termenv.Ascii},
		{"dumb terminal", map[string]string{"TERM": "dumb"}, termenv.Ascii},
		{"unset terminal", map[string]string{"TERM": ""}, termenv.Ascii},
		{"256 color", map[string]string{"TERM": "xterm-256color"}, termenv.ANSI256},
		{"basic ansi", map[string]string{"TERM": "xterm"}, termenv.ANSI},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "COLORTERM", "TERM"} {
				t.Setenv(k, "")
				os.Unsetenv(k)
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := resolveProfile(); got != tc.want {
				t.Errorf("resolveProfile() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSupportedReflectsProfile(t *testing.T) {
	if !Supported() && profile() != termenv.Ascii {
		t.Error("Supported disagrees with the resolved profile")
	}
}
