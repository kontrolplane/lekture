package export

import (
	"html/template"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// highlighter renders fenced code blocks with chroma, so exported decks keep
// the syntax highlighting the terminal renderer provides.
type highlighter struct{}

func newHighlighter() goldmark.Extender { return highlighter{} }

func (h highlighter) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(&codeRenderer{}, 100),
	))
}

type codeRenderer struct{}

func (r *codeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.render)
}

// style is resolved once; chroma's style lookup is not free.
var (
	chromaStyle     = styles.Get("catppuccin-macchiato")
	chromaFormatter = chromahtml.New(chromahtml.WithClasses(false), chromahtml.TabWidth(4))
)

func init() {
	if chromaStyle == nil {
		chromaStyle = styles.Fallback
	}
}

func (r *codeRenderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	block := node.(*ast.FencedCodeBlock)

	var code []byte
	for i := 0; i < block.Lines().Len(); i++ {
		line := block.Lines().At(i)
		code = append(code, line.Value(source)...)
	}

	language := string(block.Language(source))
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Analyse(string(code))
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, string(code))
	if err != nil {
		// Fall back to escaped plain text rather than failing the export.
		w.WriteString("<pre><code>")
		template.HTMLEscape(w, code)
		w.WriteString("</code></pre>\n")
		return ast.WalkSkipChildren, nil
	}

	if err := chromaFormatter.Format(w, chromaStyle, iterator); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkSkipChildren, nil
}
