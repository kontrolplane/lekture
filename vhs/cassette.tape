# Lekture — terminal-based presentation tool
# Generates the main demo GIF for the README.
#
# Right advances a reveal before it advances a slide, so slides with
# "<!-- pause -->" markers take more than one press.

Output assets/cassette.gif

Require go

Set FontSize 16
Set Width 1280
Set Height 800
Set Padding 20

# Build and start the presentation
Hide
Type "go build -o vhs/lekture . && ./vhs/lekture example/presentation.md"
Enter
Sleep 2s
Show

# -- Slide 1: title --
Sleep 4s

# -- Slide 2: agenda, revealed in three stages --
Right
Sleep 2500ms
Right
Sleep 2500ms
Right
Sleep 3s

# -- Slide 3: how this deck is written --
Right
Sleep 5s

# -- Slide 4: what is a vpc? --
Right
Sleep 4s

# -- Slide 5: vpc building blocks --
Right
Sleep 4s

# -- Slide 6: subnets in practice --
Right
Sleep 3s

# -- Slide 7: cidr planning --
Right
Sleep 3s

# -- Slide 8: vpc-attached lambda, two runnable blocks --
Right
Sleep 4s

# Pick the second block, then run the first
Tab
Sleep 2s
Tab
Sleep 1500ms

# Execute the Go handler
Ctrl+E
Sleep 5s
Escape
Sleep 1500ms

# -- Slides 9 to 14 --
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
Right
Sleep 3s

# -- Search --
Type "/"
Sleep 1s
Type "subnet"
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
