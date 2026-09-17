// Package export renders a presentation to a self-contained HTML document.
package export

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/kontrolplane/lekture/internal/deck"
	"github.com/kontrolplane/lekture/internal/parser"
	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	"github.com/yuin/goldmark/extension"
	gmparser "github.com/yuin/goldmark/parser"
)

// maxEmbedBytes caps how large an image may be before it is left as a
// reference rather than inlined, so one huge asset cannot produce an
// unopenable document.
const maxEmbedBytes = 8 << 20 // 8 MiB

// HTML renders the deck as a single self-contained HTML document.
//
// Progressive reveals are flattened: an exported deck is read rather than
// presented, so every step of a slide is shown at once.
//
// Raw HTML in a presentation is escaped rather than rendered. The terminal
// renderer does not honour it either, and an exported document is meant to be
// shared, so markup from a deck someone else wrote should not become live
// markup in the reader's browser.
func HTML(d *deck.Deck, w io.Writer) error {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			emoji.Emoji,
			newHighlighter(),
		),
		goldmark.WithParserOptions(gmparser.WithAutoHeadingID()),
	)

	slides := make([]slideView, 0, len(d.Slides))
	for _, s := range d.Slides {
		html, err := renderSlide(md, s, d.BaseDir)
		if err != nil {
			return fmt.Errorf("slide %d: %w", s.Index+1, err)
		}
		slides = append(slides, slideView{Index: s.Index + 1, HTML: template.HTML(html)})
	}

	title := d.Name()
	if t := firstHeading(d.Slides); t != "" {
		title = t
	}

	return pageTemplate.Execute(w, pageData{
		Title:        title,
		Author:       d.Meta.Author,
		Date:         d.Meta.Date,
		HeadingColor: template.CSS(cssColor(d.Meta.HeadingColor)),
		Slides:       slides,
		Total:        len(slides),
	})
}

type slideView struct {
	Index int
	HTML  template.HTML
}

type pageData struct {
	Title        string
	Author       string
	Date         string
	HeadingColor template.CSS
	Slides       []slideView
	Total        int
}

// renderSlide converts one slide to HTML, inlining its images.
func renderSlide(md goldmark.Markdown, s parser.Slide, baseDir string) (string, error) {
	source := s.RawMarkdown
	for _, img := range s.Images {
		uri, err := dataURI(img.Path, baseDir)
		if err != nil {
			// Leave the original reference in place; a missing image should
			// not fail the whole export.
			continue
		}
		source = strings.Replace(source,
			img.Placeholder,
			fmt.Sprintf("![%s](%s)", img.Alt, uri),
			1)
	}

	var buf bytes.Buffer
	if err := md.Convert([]byte(source), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// dataURI reads an image and encodes it for inline embedding, so the exported
// document is a single shareable file.
func dataURI(path, baseDir string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") ||
		strings.HasPrefix(path, "data:") {
		return path, nil
	}

	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(baseDir, resolved)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", resolved)
	}
	if info.Size() > maxEmbedBytes {
		return "", fmt.Errorf("%s is larger than the embed limit", resolved)
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}

	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(resolved)))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// firstHeading returns the first markdown heading in the deck, for the page
// title.
func firstHeading(slides []parser.Slide) string {
	for _, s := range slides {
		for _, line := range strings.Split(s.RawMarkdown, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") {
				return strings.TrimSpace(strings.TrimLeft(line, "#"))
			}
		}
	}
	return ""
}

// cssColor validates a hex color before it reaches the stylesheet.
func cssColor(c string) string {
	if !strings.HasPrefix(c, "#") {
		c = "#" + c
	}
	if len(c) != 4 && len(c) != 7 {
		return "#a6da95"
	}
	for _, r := range c[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return "#a6da95"
		}
	}
	return c
}
