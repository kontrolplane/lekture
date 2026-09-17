package smoke

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kontrolplane/lekture/internal/deck"
	"github.com/kontrolplane/lekture/internal/meta"
	"github.com/kontrolplane/lekture/internal/render"
)

// TestInspectExampleDeck prints the structure of the shipped example deck and
// asserts it exercises the documented features.
func TestInspectExampleDeck(t *testing.T) {
	d, err := deck.Load("../../example/presentation.md", meta.Meta{})
	if err != nil {
		t.Skip("example deck not reachable")
	}
	if d.Warning != nil {
		t.Errorf("example deck has a frontmatter warning: %v", d.Warning)
	}

	var withSteps, withCode, totalCode int
	var b strings.Builder
	for _, s := range d.Slides {
		title := "(untitled)"
		for _, l := range strings.Split(s.RawMarkdown, "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "#") {
				title = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "# "))
				break
			}
		}
		fmt.Fprintf(&b, "\n%2d  steps=%d code=%d img=%d  %s",
			s.Index+1, s.StepCount(), len(s.CodeBlocks), len(s.Images), title)
		if s.StepCount() > 1 {
			withSteps++
		}
		if len(s.CodeBlocks) > 0 {
			withCode++
		}
		totalCode += len(s.CodeBlocks)
	}
	t.Logf("slides=%d theme=%q align=%q%s", len(d.Slides), d.Meta.Theme, d.Meta.Align, b.String())

	if withSteps == 0 {
		t.Error("the example deck should demonstrate incremental reveals")
	}
	if totalCode < 2 {
		t.Error("the example deck should demonstrate more than one runnable code block")
	}
}

// TestExampleDeckFitsOnScreen keeps the shipped deck from silently overflowing.
// Content taller than the terminal is truncated with no indication, so a slide
// that grows too long loses its ending without any warning.
func TestExampleDeckFitsOnScreen(t *testing.T) {
	d, err := deck.Load("../../example/presentation.md", meta.Meta{})
	if err != nil {
		t.Skip("example deck not reachable")
	}

	// The vhs tapes record at 1280x800 with font size 16, which is close to
	// 80 columns by 40 rows.
	const cols, rows = 80, 40
	r := render.New("../../example", cols, rows, d.Meta.Theme, d.Meta.HeadingColor)

	const statusBar = 1
	budget := rows - statusBar

	for _, s := range d.Slides {
		for step := 0; step < s.StepCount(); step++ {
			out, err := r.RenderStep(s, step)
			if err != nil {
				t.Fatalf("slide %d step %d: %v", s.Index+1, step+1, err)
			}
			if n := strings.Count(out, "\n") + 1; n > budget {
				t.Errorf("slide %d (%q) step %d renders %d lines at %dx%d, which exceeds the %d available — the ending would be cut off",
					s.Index+1, headingOf(s.RawMarkdown), step+1, n, cols, rows, budget)
			}
		}
	}
}

func headingOf(md string) string {
	for _, l := range strings.Split(md, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "#") {
			return strings.TrimSpace(strings.TrimLeft(l, "# "))
		}
	}
	return "(untitled)"
}
