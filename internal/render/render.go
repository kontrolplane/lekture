package render

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/kontrolplane/lekture/internal/image"
	"github.com/kontrolplane/lekture/internal/parser"
	"github.com/kontrolplane/lekture/internal/theme"
)

type cacheKey struct {
	slideIndex int
	width      int
	height     int
}

// Renderer renders slides with markdown formatting and inline images.
type Renderer struct {
	baseDir      string
	width        int
	height       int
	themePath    string
	headingColor string

	mu    sync.Mutex
	cache map[cacheKey]string
}

// New creates a new Renderer that resolves image paths relative to baseDir.
func New(baseDir string, width, height int, themePath, headingColor string) *Renderer {
	return &Renderer{
		baseDir:      baseDir,
		width:        width,
		height:       height,
		themePath:    themePath,
		headingColor: headingColor,
		cache:        make(map[cacheKey]string),
	}
}

// SetSize updates the terminal dimensions and clears the cache.
func (r *Renderer) SetSize(width, height int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.width = width
	r.height = height
	clear(r.cache)
}

// ClearCache clears the render cache (e.g. after file reload).
func (r *Renderer) ClearCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.cache)
}

// RenderSlide renders a slide's markdown with inline images.
func (r *Renderer) RenderSlide(slide parser.Slide) (string, error) {
	r.mu.Lock()
	key := cacheKey{slideIndex: slide.Index, width: r.width, height: r.height}
	if cached, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return cached, nil
	}
	width := r.width
	height := r.height
	baseDir := r.baseDir
	themePath := r.themePath
	r.mu.Unlock()

	contentWidth := width * 3 / 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	md := slide.RawMarkdown

	// Render images and replace placeholders with tokens
	var renderedImages []string
	for i, img := range slide.Images {
		token := fmt.Sprintf("LEKIMG%dPLACEHOLDER", i)
		md = strings.Replace(md, img.Placeholder, token, 1)

		imgPath := img.Path
		if !filepath.IsAbs(imgPath) {
			imgPath = filepath.Join(baseDir, imgPath)
		}

		maxImgWidth := contentWidth
		maxImgHeight := height / 2
		rendered, err := image.Render(imgPath, maxImgWidth, maxImgHeight)
		if err != nil {
			rendered = fmt.Sprintf("[image: %s]", img.Alt)
		}
		renderedImages = append(renderedImages, rendered)
	}

	// Load base style, then apply heading differentiation.
	style := loadBaseStyle(themePath, baseDir)
	applyHeadingStyles(style, r.headingColor)

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(*style),
		glamour.WithWordWrap(contentWidth),
	)
	if err != nil {
		return "", fmt.Errorf("creating glamour renderer: %w", err)
	}

	output, err := renderer.Render(md)
	if err != nil {
		return "", fmt.Errorf("rendering markdown: %w", err)
	}

	// Replace tabs with spaces. Glamour preserves literal tabs from code
	// blocks, but lipgloss measures them as 1 cell while the terminal
	// expands them to the next 8-column tab stop. This mismatch causes
	// lines to wrap and produces ghost double headers/footers.
	output = strings.ReplaceAll(output, "\t", "    ")

	// Replace image tokens with rendered images.
	for i, rendered := range renderedImages {
		token := fmt.Sprintf("LEKIMG%dPLACEHOLDER", i)
		idx := strings.Index(output, token)
		if idx < 0 {
			continue
		}

		// Find the line boundaries around the token.
		lineStart := strings.LastIndex(output[:idx], "\n") + 1
		lineEnd := strings.Index(output[idx:], "\n")
		if lineEnd < 0 {
			lineEnd = len(output)
		} else {
			lineEnd += idx
		}

		// Extract the leading visual whitespace on the token's line,
		// skipping over any ANSI escape sequences glamour may insert.
		prefix := extractLeadingWhitespace(output[lineStart:idx])

		// Prefix every image line so all rows share the same indentation.
		imageLines := strings.Split(rendered, "\n")
		for j := range imageLines {
			imageLines[j] = prefix + imageLines[j]
		}
		rendered = strings.Join(imageLines, "\n")

		// Replace the entire line containing the token with the image block.
		output = output[:lineStart] + rendered + output[lineEnd:]
	}

	output = strings.TrimSpace(output)

	r.mu.Lock()
	r.cache[key] = output
	r.mu.Unlock()

	return output, nil
}

func loadBaseStyle(themePath, baseDir string) *ansi.StyleConfig {
	if themePath != "" {
		style, err := theme.Load(themePath, baseDir)
		if err == nil {
			return style
		}
	}
	return autoStyle()
}

// colorize wraps each line of text in ANSI foreground color codes.
func colorize(text, hexColor string) string {
	r, g, b := parseHex(hexColor)
	prefix := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	reset := "\x1b[0m"
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line + reset
	}
	return strings.Join(lines, "\n")
}

// autoStyle returns the default dark or light style based on terminal background.
func autoStyle() *ansi.StyleConfig {
	// glamour.WithAutoStyle() picks dark/light internally. We replicate the
	// logic here so we get a mutable StyleConfig to customize.
	s := styles.DarkStyleConfig
	return &s
}

func stringPtr(s string) *string { return &s }
func boolPtr(b bool) *bool       { return &b }
func uintPtr(u uint) *uint       { return &u }

// extractLeadingWhitespace returns the leading spaces/tabs from s,
// skipping ANSI escape sequences (CSI: \x1b[...m) that glamour inserts.
func extractLeadingWhitespace(s string) string {
	var ws strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Skip the entire CSI sequence: \x1b[ ... <final byte>
			i += 2
			for i < len(s) && s[i] < 0x40 || s[i] > 0x7e {
				i++
			}
			if i < len(s) {
				i++ // skip final byte (e.g. 'm')
			}
			continue
		}
		if s[i] == ' ' || s[i] == '\t' {
			ws.WriteByte(s[i])
			i++
			continue
		}
		break
	}
	return ws.String()
}

// applyHeadingStyles differentiates H1–H6 using the given base color.
// Lower heading levels progressively fade toward gray.
func applyHeadingStyles(s *ansi.StyleConfig, color string) {
	s.Heading.StylePrimitive.BlockSuffix = "\n"
	s.Heading.StylePrimitive.Bold = boolPtr(true)

	faded := blendHex(color, "#888888", 0.45)
	dim := blendHex(color, "#888888", 0.7)

	// H1: bold, colored background, wide horizontal padding, generous vertical
	// spacing so title slides feel dominant and presentation-like.
	s.H1 = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix:          "    ",
			Suffix:          "    ",
			Color:           stringPtr("#1e1e2e"),
			BackgroundColor: stringPtr(color),
			Bold:            boolPtr(true),
			Upper:           boolPtr(true),
			BlockPrefix:     "\n\n\n\n\n\n",
			BlockSuffix:     "\n\n\n",
		},
	}

	// H2: bold, full color — section heading
	s.H2 = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Color:       stringPtr(color),
			Bold:        boolPtr(true),
			BlockSuffix: "\n",
		},
	}

	// H3: bold, faded — subsection
	s.H3 = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Color:       stringPtr(faded),
			Bold:        boolPtr(true),
			BlockSuffix: "\n",
		},
	}

	// H4: bold italic, dimmer
	s.H4 = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Color:       stringPtr(dim),
			Bold:        boolPtr(true),
			Italic:      boolPtr(true),
			BlockSuffix: "\n",
		},
	}

	// H5/H6: italic, gray
	s.H5 = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Color:       stringPtr("#888888"),
			Italic:      boolPtr(true),
			BlockSuffix: "\n",
		},
	}
	s.H6 = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Color:       stringPtr("#777777"),
			Italic:      boolPtr(true),
			BlockSuffix: "\n",
		},
	}
}

// blendHex linearly interpolates between two hex colors.
// t=0 returns a, t=1 returns b.
func blendHex(a, b string, t float64) string {
	ar, ag, ab := parseHex(a)
	br, bg, bb := parseHex(b)
	lerp := func(x, y uint8, t float64) uint8 {
		return uint8(float64(x)*(1-t) + float64(y)*t)
	}
	return fmt.Sprintf("#%02x%02x%02x", lerp(ar, br, t), lerp(ag, bg, t), lerp(ab, bb, t))
}

func parseHex(hex string) (r, g, b uint8) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return 0x88, 0x88, 0x88
	}
	val := func(s string) uint8 {
		n, _ := fmt.Sscanf(s, "%02x", new(int))
		if n == 0 {
			return 0x88
		}
		var v int
		fmt.Sscanf(s, "%02x", &v)
		return uint8(v)
	}
	return val(hex[0:2]), val(hex[2:4]), val(hex[4:6])
}

