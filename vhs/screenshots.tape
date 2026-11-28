# Lekture — screenshot captures for the README.
#
# Slide map (example/presentation.md):
#   1 title                              5 images
#   2 what this deck covers (3 reveals)  6 revealing a slide in stages (3 reveals)
#   3 a deck is one markdown file        7 running code (2 code blocks)
#   4 what a slide can hold             16 thank you
#
# Right advances a reveal before it advances a slide, so a slide with three
# reveals needs three presses to pass.

Require go

Set FontSize 16
Set Width 1280
Set Height 800
Set Padding 20
Set WaitTimeout 1m

Hide
Type "go build -o vhs/lekture . && ./vhs/lekture example/presentation.md"
Enter
Sleep 2s
Show

# -- Screenshot 1: a slide revealed in stages --
# slide 1 -> 2 (first reveal), then one more to reveal the middle pair.
Right
Sleep 1s
Right
Sleep 1500ms
Screenshot assets/pages/slide/reveal.png
Sleep 500ms

# -- Screenshot 2: slide overview --
# finish the reveals (1 press), past "a deck is one markdown file" (1), to slide 4.
Hide
Right
Right
Show
Right
Sleep 1500ms
Screenshot assets/pages/slide/overview.png
Sleep 500ms

# -- Screenshot 3: code block with syntax highlighting --
# slide 5, the three reveals of slide 6, then slide 7.
Hide
Right
Right
Right
Right
Right
Show
Sleep 1500ms
Screenshot assets/pages/slide/code-block.png
Sleep 500ms

# -- Screenshot 4: code execution --
Ctrl+E
Wait+Screen /exit 0/
Sleep 1s
Screenshot assets/pages/slide/code-execution.png
Sleep 500ms
Escape
Sleep 500ms

# -- Screenshot 5: the keymap overlay --
Type "?"
Sleep 1500ms
Screenshot assets/pages/slide/help.png
Sleep 500ms
Escape
Sleep 500ms

Type "q"
Sleep 500ms
