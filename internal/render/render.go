package render

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/kontrolplane/lekture/internal/image"
	"github.com/kontrolplane/lekture/internal/parser"
	"github.com/kontrolplane/lekture/internal/theme"
)

type cacheKey struct {
	slideIndex int
	step       int
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

	// styleCache holds the parsed theme so a remote theme is fetched once
	// rather than on every cache miss (SetSize clears the render cache).
	styleLoaded bool
	styleKey    string
	styleValue  *ansi.StyleConfig

	// themeErr records why a configured theme failed to load, so the failure
	// can be reported instead of silently falling back to the default.
	themeErr error
}

// ThemeError reports why the configured theme could not be loaded, if it
// could not. Rendering falls back to the default style in that case.
func (r *Renderer) ThemeError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.themeErr
}

// New creates a new Renderer that resolves image paths relative to baseDir.
func New(baseDir string, width, height int, themePath, headingColor string) *Renderer {
	r := &Renderer{
		baseDir:      baseDir,
		width:        width,
		height:       height,
		themePath:    themePath,
		headingColor: headingColor,
		cache:        make(map[cacheKey]string),
	}
	// Resolve the theme now rather than on first render, so a bad theme is
	// reported before the presentation starts instead of silently falling
	// back to the default.
	r.baseStyle()
	return r
}

// SetSize updates the terminal dimensions and clears the cache.
func (r *Renderer) SetSize(width, height int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.width == width && r.height == height {
		return
	}
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

// SetTheme updates the theme and heading color, e.g. after the presentation's
// frontmatter changed on disk. It clears both the render and style caches.
func (r *Renderer) SetTheme(themePath, headingColor string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.themePath == themePath && r.headingColor == headingColor {
		return
	}
	r.themePath = themePath
	r.headingColor = headingColor
	r.styleLoaded = false
	r.styleKey = ""
	r.styleValue = nil
	r.themeErr = nil
	clear(r.cache)
}

// RenderSlide renders a slide's markdown with inline images.
func (r *Renderer) RenderSlide(slide parser.Slide) (string, error) {
	return r.RenderStep(slide, slide.StepCount()-1)
}

// RenderStep renders one progressive reveal of a slide.
func (r *Renderer) RenderStep(slide parser.Slide, step int) (string, error) {
	r.mu.Lock()
	key := cacheKey{slideIndex: slide.Index, step: step, width: r.width, height: r.height}
	if cached, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return cached, nil
	}
	width := r.width
	height := r.height
	baseDir := r.baseDir
	headingColor := r.headingColor
	r.mu.Unlock()

	contentWidth := width * 3 / 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	md := slide.Step(step)

	// Render images and replace placeholders with tokens
	// Each entry pairs the token substituted into the markdown with the
	// rendered image, so skipping an image cannot misalign the two.
	type placedImage struct{ token, rendered string }
	var renderedImages []placedImage

	for i, img := range slide.Images {
		if !strings.Contains(md, img.Placeholder) {
			// The image belongs to a later reveal.
			continue
		}
		token := fmt.Sprintf("LEKIMG%dPLACEHOLDER", i)
		md = strings.Replace(md, img.Placeholder, token, 1)

		imgPath := img.Path
		if !filepath.IsAbs(imgPath) {
			imgPath = filepath.Join(baseDir, imgPath)
		}

		maxImgWidth := contentWidth
		maxImgHeight := height / 2

		var rendered string
		if !image.Supported() {
			// Without color the half-block glyphs convey nothing, so show
			// the alt text instead of a screenful of noise.
			rendered = fmt.Sprintf("[image: %s]", img.Alt)
		} else if out, err := image.Render(imgPath, maxImgWidth, maxImgHeight); err != nil {
			rendered = fmt.Sprintf("[image: %s — %v]", img.Alt, err)
		} else {
			rendered = out
		}
		renderedImages = append(renderedImages, placedImage{token: token, rendered: rendered})
	}

	// Load base style, then apply heading differentiation. The base style is
	// memoized so a remote theme is fetched once, not on every cache miss.
	style := r.baseStyle()
	applyHeadingStyles(style, headingColor)

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(*style),
		glamour.WithWordWrap(contentWidth),
		glamour.WithEmoji(),
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
	for _, img := range renderedImages {
		token, rendered := img.token, img.rendered
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

	output = hardWrap(output, contentWidth)
	output = strings.TrimSpace(output)

	r.mu.Lock()
	r.cache[key] = output
	r.mu.Unlock()

	return output, nil
}

// baseStyle returns a copy of the theme”'s style config, loading and memoizing
// it on first use. A copy is returned because callers mutate it.
func (r *Renderer) baseStyle() *ansi.StyleConfig {
	r.mu.Lock()
	themePath := r.themePath
	baseDir := r.baseDir
	if r.styleLoaded && r.styleKey == themePath {
		style := *r.styleValue
		r.mu.Unlock()
		return &style
	}
	r.mu.Unlock()

	loaded, err := loadBaseStyle(themePath, baseDir)

	r.mu.Lock()
	r.styleLoaded = true
	r.styleKey = themePath
	r.styleValue = loaded
	r.themeErr = err
	r.mu.Unlock()

	style := *loaded
	return &style
}

func loadBaseStyle(themePath, baseDir string) (*ansi.StyleConfig, error) {
	if themePath == "" {
		return autoStyle(), nil
	}
	style, err := theme.Load(themePath, baseDir)
	if err != nil {
		return autoStyle(), fmt.Errorf("theme %q: %w (using default)", themePath, err)
	}
	return style, nil
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

// hardWrap breaks any line still wider than width after glamour has run.
// Word wrapping only breaks on spaces, so a single long token (a bare URL, a
// long identifier) is emitted whole and would otherwise run past the edge of
// the terminal and wrap at the terminal level, corrupting the layout.
func hardWrap(s string, width int) string {
	if width < 1 {
		return s
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		// Cheap byte-length precondition: a line can only be too wide if it
		// also has more bytes than the target width.
		if len(line) <= width || xansi.StringWidth(line) <= width {
			out = append(out, line)
			continue
		}
		for xansi.StringWidth(line) > width {
			out = append(out, xansi.Truncate(line, width, ""))
			line = xansi.TruncateLeft(line, width, "")
		}
		if strings.TrimSpace(xansi.Strip(line)) != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// extractLeadingWhitespace returns the leading spaces/tabs from s,
// skipping ANSI escape sequences (CSI: \x1b[...m) that glamour inserts.
func extractLeadingWhitespace(s string) string {
	var ws strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Skip the entire CSI sequence: \x1b[ ... <final byte>
			i += 2
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
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
			Color:           stringPtr(contrastColor(color)),
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

// contrastColor returns a foreground color readable against the given
// background, using the WCAG relative-luminance formula.
func contrastColor(bg string) string {
	r, g, b := parseHex(bg)
	lum := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	if lum > 140 {
		return "#1e1e2e"
	}
	return "#f5f5f5"
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
		v, err := strconv.ParseUint(s, 16, 8)
		if err != nil {
			return 0x88
		}
		return uint8(v)
	}
	return val(hex[0:2]), val(hex[2:4]), val(hex[4:6])
}
