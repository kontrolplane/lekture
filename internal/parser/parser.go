package parser

import (
	"os"
	"regexp"
	"strings"
)

var imageRegex = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
var codeBlockRegex = regexp.MustCompile("(?m)^```(\\w+)?\\n([\\s\\S]*?)^```\\s*$")

// ImageRef represents a markdown image reference found in a slide.
type ImageRef struct {
	Alt         string
	Path        string
	Placeholder string // the full markdown text, e.g. "![alt](path)"
}

// CodeBlock represents a fenced code block found in a slide.
type CodeBlock struct {
	Language string
	Code     string
}

// Slide represents a single presentation slide.
type Slide struct {
	RawMarkdown string
	Images      []ImageRef
	CodeBlocks  []CodeBlock
	Index       int
}

// ParseFile reads a markdown file and parses it into slides.
func ParseFile(path string) ([]Slide, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseContent(string(data)), nil
}

// ParseContent splits markdown content into slides on "---" separators
// and extracts image references from each slide.
func ParseContent(content string) []Slide {
	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")

	parts := strings.Split(content, "\n---\n")

	var slides []Slide
	for i, part := range parts {
		md := strings.TrimSpace(part)
		if md == "" {
			continue
		}
		slides = append(slides, Slide{
			RawMarkdown: md,
			Images:      extractImages(md),
			CodeBlocks:  extractCodeBlocks(md),
			Index:       i,
		})
	}

	// Re-index after filtering empty slides
	for i := range slides {
		slides[i].Index = i
	}

	return slides
}

func extractCodeBlocks(markdown string) []CodeBlock {
	matches := codeBlockRegex.FindAllStringSubmatch(markdown, -1)
	var blocks []CodeBlock
	for _, m := range matches {
		blocks = append(blocks, CodeBlock{
			Language: m[1],
			Code:     strings.TrimRight(m[2], "\n"),
		})
	}
	return blocks
}

func extractImages(markdown string) []ImageRef {
	matches := imageRegex.FindAllStringSubmatch(markdown, -1)
	var refs []ImageRef
	for _, m := range matches {
		refs = append(refs, ImageRef{
			Alt:         m[1],
			Path:        m[2],
			Placeholder: m[0],
		})
	}
	return refs
}
