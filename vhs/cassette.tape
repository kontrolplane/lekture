# Lekture — terminal-based presentation tool
# Generates the demo GIF of example/presentation.md, the deck that explains
# lekture itself.
#
# Right advances a reveal before it advances a slide, so slides with
# "<!-- pause -->" markers take more than one press.
#
# Slide map:
#   1 title                              7 running code (2 code blocks)
#   2 what this deck covers (3 reveals)  8 output while it runs
#   3 a deck is one markdown file        9 moving around      13 themes
#   4 what a slide can hold             10 searching          14 on a big screen
#   5 images                            11 frontmatter        15 not in the room
#   6 revealing a slide (3 reveals)     12 your own defaults  16 thank you

Output assets/cassette.gif

Require go

Set FontSize 16
Set Width 1280
Set Height 800
Set Padding 20
Set WaitTimeout 1m

# Build and start the presentation
Hide
Type "go build -o vhs/lekture . && ./vhs/lekture example/presentation.md"
Enter
Sleep 2s
Show

# -- Slide 1: title --
Sleep 3s

# -- Slide 2: what this deck covers, revealed in three stages --
Right
Sleep 2500ms
Right
Sleep 2500ms
Right
Sleep 3s

# -- Slide 3: a deck is one markdown file --
Right
Sleep 5s

# -- Slide 4: what a slide can hold --
Right
Sleep 4s

# -- Slide 5: images --
Right
Sleep 4s

# -- Slide 6: revealing a slide in stages --
Right
Sleep 2500ms
Right
Sleep 2500ms
Right
Sleep 3s

# -- Slide 7: running code, two runnable blocks --
Right
Sleep 4s

# Move the mark to the second block, and back to the first
Tab
Sleep 2s
Tab
Sleep 1500ms

# Run the Go program
Ctrl+E
Wait+Screen /exit 0/
Sleep 4s
Escape
Sleep 1s

# -- Slide 8: output while it runs --
Right
Sleep 3s
Ctrl+E
Wait+Screen /exit 0/
Sleep 2s
Escape
Sleep 1s

# -- Slides 9 to 15 --
Right
Sleep 4s
Right
Sleep 4s
Right
Sleep 3s
Right
Sleep 3s
Right
Sleep 3s
Right
Sleep 3s
Right
Sleep 3s

# -- Search --
Type "/"
Sleep 1s
Type "stage"
Sleep 1s
Enter
Sleep 3s

# Next and previous match
Ctrl+N
Sleep 2500ms
Ctrl+P
Sleep 2500ms
Escape
Sleep 1500ms

# -- Keymap overlay --
Type "?"
Sleep 4s
Escape
Sleep 1s

# -- Vim navigation --
Type "gg"
Sleep 2500ms

Shift+G
Sleep 2500ms

Type "10"
Sleep 500ms
Shift+G
Sleep 3s

# Exit
Type "q"
Sleep 2s
