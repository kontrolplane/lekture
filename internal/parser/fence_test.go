package parser

import "testing"

func TestSeparatorInsideCodeFenceDoesNotSplit(t *testing.T) {
	content := "# Slide\n\n```yaml\n---\nauthor: levi\n---\n```\n\n---\n\n# Two\n"
	slides := ParseContent(content)
	if len(slides) != 2 {
		t.Fatalf("got %d slides, want 2", len(slides))
	}
	if got := slides[0].RawMarkdown; !contains(got, "author: levi") {
		t.Errorf("first slide lost its fenced content:\n%s", got)
	}
}

func TestTildeFenceAndInfoString(t *testing.T) {
	cases := []struct {
		name, md, wantLang, wantCode string
	}{
		{"backtick", "```go\nx := 1\n```", "go", "x := 1"},
		{"info attrs", "```go {highlight=1}\nx := 1\n```", "go", "x := 1"},
		{"tilde", "~~~python\nprint(1)\n~~~", "python", "print(1)"},
		{"no lang", "```\nplain\n```", "", "plain"},
		{"trailing space", "```sh \necho hi\n```", "sh", "echo hi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks := extractCodeBlocks(tc.md)
			if len(blocks) != 1 {
				t.Fatalf("got %d blocks, want 1", len(blocks))
			}
			if blocks[0].Language != tc.wantLang {
				t.Errorf("language = %q, want %q", blocks[0].Language, tc.wantLang)
			}
			if blocks[0].Code != tc.wantCode {
				t.Errorf("code = %q, want %q", blocks[0].Code, tc.wantCode)
			}
		})
	}
}

func TestSeparatorWithTrailingWhitespace(t *testing.T) {
	slides := ParseContent("# One\n\n---  \n\n# Two\n")
	if len(slides) != 2 {
		t.Fatalf("got %d slides, want 2", len(slides))
	}
}

func TestImagePathWithParens(t *testing.T) {
	refs := extractImages("![shot](Screenshot (1).png)")
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	if refs[0].Path != "Screenshot (1).png" {
		t.Errorf("path = %q, want %q", refs[0].Path, "Screenshot (1).png")
	}
}

func TestImageWithTitle(t *testing.T) {
	refs := extractImages(`![alt](pic.png "a title")`)
	if len(refs) != 1 || refs[0].Path != "pic.png" {
		t.Fatalf("refs = %+v, want one ref with path pic.png", refs)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
