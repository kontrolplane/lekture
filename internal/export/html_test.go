package export

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kontrolplane/lekture/internal/deck"
	"github.com/kontrolplane/lekture/internal/meta"
)

func exportString(t *testing.T, md, baseDir string) string {
	t.Helper()
	d := deck.Parse(md, baseDir, meta.Meta{})
	var buf bytes.Buffer
	if err := HTML(d, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestExportProducesOneSectionPerSlide(t *testing.T) {
	out := exportString(t, "# one\n\n---\n\n# two\n\n---\n\n# three", t.TempDir())
	if n := strings.Count(out, `<section class="slide"`); n != 3 {
		t.Errorf("got %d slide sections, want 3", n)
	}
	if !strings.Contains(out, "<!DOCTYPE html>") {
		t.Error("missing doctype")
	}
}

// TestExportIsSelfContained is the property that makes an exported deck
// shareable: it must not reference anything outside the file.
func TestExportIsSelfContained(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "pic.png"))
	out := exportString(t, "# one\n\n![a picture](pic.png)\n", dir)

	refs := regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllStringSubmatch(out, -1)
	for _, r := range refs {
		v := r[1]
		if strings.HasPrefix(v, "data:") || strings.HasPrefix(v, "#") {
			continue
		}
		t.Errorf("external reference in exported document: %q", v)
	}
	if !strings.Contains(out, "data:image/png;base64,") {
		t.Error("the image was not embedded")
	}
}

func TestExportKeepsMissingImageAsReference(t *testing.T) {
	// A missing image must not fail the export.
	out := exportString(t, "# one\n\n![gone](nope.png)\n", t.TempDir())
	if !strings.Contains(out, "nope.png") {
		t.Error("expected the original reference to be preserved")
	}
}

func TestExportHighlightsCode(t *testing.T) {
	out := exportString(t, "# one\n\n```go\nfunc main() {}\n```\n", t.TempDir())
	if !strings.Contains(out, "<pre") {
		t.Fatal("no code block rendered")
	}
	if !strings.Contains(out, "color:#") {
		t.Error("code block was not syntax highlighted")
	}
}

func TestExportRendersGFM(t *testing.T) {
	md := "# one\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n\n- [x] done\n- [ ] todo\n\n~~struck~~\n"
	out := exportString(t, md, t.TempDir())
	for _, want := range []string{"<table>", "checkbox", "<del>"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the output", want)
		}
	}
}

// TestExportFlattensReveals documents the deliberate choice that an exported
// deck is read rather than presented, so every reveal is visible.
func TestExportFlattensReveals(t *testing.T) {
	out := exportString(t, "# one\n\nfirst\n\n<!-- pause -->\n\nsecond\n", t.TempDir())
	if !strings.Contains(out, "first") || !strings.Contains(out, "second") {
		t.Error("both reveals should appear in the export")
	}
	if strings.Contains(out, "pause") {
		t.Error("the reveal marker leaked into the output")
	}
}

func TestExportEscapesTitle(t *testing.T) {
	out := exportString(t, "# <script>alert(1)</script>\n", t.TempDir())
	if strings.Contains(out, "<title><script>") {
		t.Error("title was not escaped")
	}
}

func TestExportRejectsUnsafeHeadingColor(t *testing.T) {
	d := deck.Parse("# one", t.TempDir(), meta.Meta{})
	d.Meta.HeadingColor = "red; } body { display:none } :root{"
	var buf bytes.Buffer
	if err := HTML(d, &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "display:none") {
		t.Error("an unsafe heading color reached the stylesheet")
	}
}

func TestCSSColorValidation(t *testing.T) {
	cases := map[string]string{
		"#a6da95": "#a6da95",
		"a6da95":  "#a6da95",
		"#fff":    "#fff",
		"bogus":   "#a6da95",
		"":        "#a6da95",
		"#12345":  "#a6da95",
	}
	for in, want := range cases {
		if got := cssColor(in); got != want {
			t.Errorf("cssColor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExportUsesFirstHeadingAsTitle(t *testing.T) {
	out := exportString(t, "# my talk\n\n---\n\n## later", t.TempDir())
	if !strings.Contains(out, "<title>my talk</title>") {
		t.Error("expected the first heading as the document title")
	}
}

func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestExportEscapesRawHTML keeps markup from a deck out of the shared
// document, matching what the terminal renderer does.
func TestExportEscapesRawHTML(t *testing.T) {
	out := exportString(t, "# one\n\n<img src=x onerror=alert(1)>\n\n<script>alert(2)</script>\n", t.TempDir())
	if strings.Contains(out, "onerror=alert(1)") {
		t.Error("raw html was rendered rather than escaped")
	}
	if strings.Contains(out, "<script>alert(2)</script>") {
		t.Error("a script tag from the deck reached the document")
	}
}
