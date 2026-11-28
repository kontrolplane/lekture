package smoke

import (
	"os"
	"strings"
	"testing"

	"github.com/kontrolplane/lekture/internal/meta"
	"github.com/kontrolplane/lekture/internal/parser"
	"github.com/kontrolplane/lekture/internal/render"
)

// exampleDecks are the decks shipped in example/, each of which has to parse
// and render.
var exampleDecks = []string{"presentation.md", "kubernetes.md", "helm.md"}

func TestExampleDeckRenders(t *testing.T) {
	for _, name := range exampleDecks {
		t.Run(name, func(t *testing.T) { renderExampleDeck(t, name) })
	}
}

func renderExampleDeck(t *testing.T, name string) {
	t.Helper()
	data, err := os.ReadFile("../../example/" + name)
	if err != nil {
		t.Skip("example deck not reachable")
	}
	m, rest, err := meta.Extract(string(data))
	if err != nil {
		t.Fatalf("frontmatter: %v", err)
	}
	slides := parser.ParseContent(rest)
	t.Logf("slides=%d author=%q date=%q paging=%q", len(slides), m.Author, m.Date, m.Paging)
	if len(slides) == 0 {
		t.Fatal("no slides")
	}
	r := render.New("../../example", 120, 40, m.Theme, m.HeadingColor)
	code := 0
	for _, s := range slides {
		out, err := r.RenderSlide(s)
		if err != nil {
			t.Fatalf("slide %d: %v", s.Index, err)
		}
		if strings.Contains(out, "LEKIMG") {
			t.Errorf("slide %d leaked an image placeholder", s.Index)
		}
		code += len(s.CodeBlocks)
	}
	t.Logf("code blocks found: %d", code)
}
