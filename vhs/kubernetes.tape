# Lekture — a recording of example/kubernetes.md, run for real.
#
# Every command in the recording runs against a k3d cluster named "lekture"
# that the deck creates on slide 4 and deletes on slide 13. The recording
# points KUBECONFIG at a temporary file, so the kubeconfig of whoever runs
# the tape is never touched.
#
# The waits match on the output of a command, so a slow machine makes a
# longer recording rather than a cut-off one. Where the cluster needs a
# moment (a pod pulling its image), the tape waits off the record between
# Hide and Show, so the recording does not sit still.
#
# Slide map:
#   1 title              6 ask for an outcome     11 one address (3 blocks)
#   2 looking after      7 what the manifest says 12 a new version (3 blocks)
#   3 a cluster          8 the loop               13 cleanup (2 reveals)
#   4 start one (2)      9 break it (2 blocks)    14 thank you
#   5 a pod (3 blocks)  10 more copies (2 blocks)

Output assets/kubernetes.gif

Require go
Require docker
Require k3d
Require kubectl

Set FontSize 16
Set Width 1280
Set Height 800
Set Padding 20
Set WaitTimeout 3m

# Build and start the presentation
Hide
Type "export KUBECONFIG=$(mktemp -d)/config && go build -o vhs/lekture . && ./vhs/lekture example/kubernetes.md"
Enter
Sleep 3s
Show

# -- Slide 1: title --
Sleep 3s

# -- Slide 2: containers need looking after --
Right
Sleep 4s

# -- Slide 3: a cluster --
Right
Sleep 4s

# -- Slide 4: start one, created while you watch --
Right
Sleep 2500ms
Ctrl+E
Wait+Screen /created successfully/
Sleep 2500ms
Escape
Sleep 500ms

# The second block lists the nodes
Tab
Sleep 1s
Ctrl+E
Wait+Screen /control-plane/
Sleep 3s
Escape
Sleep 500ms

Hide
Sleep 10s
Show

# -- Slide 5: a pod on its own --
Right
Sleep 2500ms
Ctrl+E
Wait+Screen /web created/
Sleep 2s
Escape

# The pod pulls its image
Hide
Sleep 15s
Show

Tab
Sleep 1s
Ctrl+E
Wait+Screen /web +1.1 +Running/
Sleep 2500ms
Escape
Sleep 500ms

# Delete it
Tab
Sleep 1s
Ctrl+E
Wait+Screen /deleted/
Sleep 2s
Escape
Sleep 500ms

# Tab wraps around to the list: nothing is left
Tab
Tab
Sleep 1s
Ctrl+E
Wait+Screen /No resources/
Sleep 3s
Escape
Sleep 500ms

# -- Slide 6: ask for an outcome --
Right
Sleep 3s
Ctrl+E
Wait+Screen /web created/
Sleep 2s
Escape

Hide
Sleep 15s
Show

# -- Slide 7: what the manifest says --
Right
Sleep 1500ms
Ctrl+E
Wait+Screen /web +3.3/
Sleep 4s
Escape
Sleep 4s

# -- Slide 8: the loop --
Right
Sleep 5s

# -- Slide 9: break it --
Right
Sleep 2500ms
Ctrl+E
Wait+Screen /deleted/
Sleep 2500ms
Escape

Hide
Sleep 8s
Show

Tab
Sleep 1s
Ctrl+E
Wait+Screen /Running/
Sleep 4s
Escape
Sleep 500ms

# -- Slide 10: more copies --
Right
Sleep 2s
Ctrl+E
Wait+Screen /scaled/
Sleep 2s
Escape

Hide
Sleep 8s
Show

Tab
Sleep 1s
Ctrl+E
Wait+Screen /POD +NODE/
Sleep 4s
Escape
Sleep 500ms

# -- Slide 11: one address for all of them --
Right
Sleep 3s
Ctrl+E
Wait+Screen /exposed/
Sleep 2s
Escape
Sleep 500ms

Tab
Sleep 1s
Ctrl+E
Wait+Screen /ENDPOINTS/
Sleep 3500ms
Escape
Sleep 500ms

# Ask the service for a page, then again: another pod answers
Tab
Sleep 1s
Ctrl+E
Wait+Screen /"version"/
Sleep 3s
Type "r"
Sleep 1s
Wait+Screen /"version"/
Sleep 3s
Escape
Sleep 500ms

# -- Slide 12: a new version --
Right
Sleep 3s
Ctrl+E
Wait+Screen /image updated/
Sleep 1500ms
Escape
Sleep 500ms

# Follow the rollout as it happens
Tab
Sleep 500ms
Ctrl+E
Wait+Screen /successfully rolled out/
Sleep 2500ms
Escape
Sleep 500ms

# The same request is now answered by the new version
Tab
Sleep 1s
Ctrl+E
Wait+Screen /"version"/
Sleep 4s
Escape
Sleep 500ms

# -- Slide 13: cleanup --
Right
Sleep 2s
Ctrl+E
Wait+Screen /Successfully deleted/
Sleep 2500ms
Escape
Sleep 1s
Right
Sleep 2500ms

# -- Slide 14: thank you --
Right
Sleep 3s

# Exit
Type "q"
Sleep 1s
