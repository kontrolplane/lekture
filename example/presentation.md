---
author: levi van noort
date: YYYY-MM-dd
paging: "%d / %d"
headingColor: "#F4E8C1"
# theme: kontrolplane   # a bundled theme, resolved by name from anywhere
# align: center         # or the default, top-left
---

# lekture

### markdown slides, presented in your terminal

---

## what this deck covers

1. **writing** — one markdown file, and what a slide can hold
2. **revealing** — showing a slide in stages

<!-- pause -->

3. **running code** — executing a block without leaving the slide
4. **moving around** — keys, counts and search

<!-- pause -->

5. **configuring** — frontmatter, your own defaults, themes
6. **sharing** — a big screen, ssh, html and plain text

> every slide is written with the thing it describes. keep `example/presentation.md` open next to it.

---

## a deck is one markdown file

```markdown
---
author: your name
paging: "%d / %d"
---

# first slide

---

## second slide

- shown straight away

<!-- pause -->

- revealed on the next keypress
```

- a line that holds only `---` starts the next slide
- the block between the first two is **frontmatter**: settings for the deck
- a `---` **inside** a fenced block is content, which is why this slide holds together

---

## what a slide can hold

whatever markdown can say: **bold**, *italic*, `inline code` and [links](https://github.com/kontrolplane/lekture).

| element | written as                         |
|---------|------------------------------------|
| heading | `# title` down to `###### label`   |
| list    | `- item` or `1. item`              |
| table   | pipes and dashes, like this one    |
| quote   | `> text`                           |
| code    | three backticks and a language     |
| image   | a link with `!` in front of it     |

- an image is drawn on the slide itself: the next slide has one

> a quote sets one sentence apart. use it for the one to remember.

---

## images

![kontrolplane](logo.png)

- written as a link with `!` in front of it: the alt text, then the file
- a png, jpeg or gif, found from where the deck is
- drawn as large as fits the slide's width and half the terminal's height
- a terminal without colour shows the alt text instead: `kontrolplane`

---

## revealing a slide in stages

put `<!-- pause -->` on a line of its own. everything after it waits for a keypress.

- this line was here from the start

<!-- pause -->

- this one took a keypress, and the counter now reads `(2/3)`

<!-- pause -->

- this is the last stage, so the next keypress moves to the next slide

> going back lands on the *last* stage of a slide, so nothing the audience saw is hidden again.

---

## running code

`ctrl+e` runs the marked block and shows what it prints. `r` runs it again, `esc` closes the output.

```go
package main

import (
	"fmt"
	"runtime"
)

func main() {
	fmt.Println("hello from a slide")
	fmt.Printf("   go:        %s\n", runtime.Version())
	fmt.Printf("   platform:  %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
```

more than one block on a slide? `tab` moves the mark to the next one:

```bash
echo "a block runs in a directory of its own: $(pwd)"
```

---

## output while it runs

the output appears as it is written, so a slow command shows its progress.

```bash
for step in 1 2 3 4 5; do
  echo "step $step of 5"
  sleep 1
done
```

- runs go, python, javascript, ruby, lua, elixir, bash, c and rust
- a block is stopped after **10 seconds**: raise that with `execTimeout` in the frontmatter
- `esc` on a block that is still running cancels it
- a fence in any other language is highlighted, and `tab` skips it

> running a block runs it on your machine. treat a deck you did not write like a script you did not write.

---

## moving around

| key                           | does                                 |
|-------------------------------|--------------------------------------|
| `→` `l` `j` `space` `enter`   | next stage, then the next slide      |
| `←` `h` `k` `backspace`       | previous stage, then the slide before|
| `gg` `home`                   | first slide                          |
| `G` `end`                     | last slide                           |
| `5j` `3k`                     | move that many slides                |
| `12G`                         | jump to slide 12                     |
| `?`                           | every key, on top of the slide       |
| `q`                           | quit                                 |

> a count or a jump moves whole slides, and starts each at its first stage.

---

## searching

press `/`, type what you are looking for, press `enter`.

| key      | does                                   |
|----------|----------------------------------------|
| `ctrl+n` | next slide that matches                |
| `ctrl+p` | previous slide that matches            |
| `esc`    | clear the search                       |

- the search is a **regular expression**: `ssh|html` finds either word
- end it with `/i` to ignore case: `markdown/i`

> try it here: `/stage` and then `ctrl+n` a few times.

---

## frontmatter

```yaml
---
author: your name          # left of the status bar
date: YYYY-MM-dd           # next to it, today in this format
paging: "%d / %d"          # the slide counter, on the right
theme: kontrolplane        # a bundled name, a file or a url
align: center              # or top-left, which is the default
headingColor: "#F4E8C1"    # the headings take their colours from it
execTimeout: 30s           # how long one code block may run
timer: elapsed             # time since the first slide, or clock
---
```

- every field is optional, and a value that makes no sense falls back to its default
- save the file and the deck reloads, on the slide you were on

---

## your own defaults

the same fields, for every deck you present, in `~/.config/lekture/config.yaml`:

```yaml
author: your name
theme: kontrolplane
align: center
```

```text
built-in defaults   ▶   config file   ▶   frontmatter
```

- each one overrides the one to its left, so a deck always wins over your defaults
- a key the config file does not know is an error, so a typo is reported instead of ignored

---

## themes

| `theme:`               | gives you                                   |
|------------------------|---------------------------------------------|
| `default`              | the built-in look, which this deck uses     |
| `kontrolplane`         | navy, cream and a goldenrod accent          |
| `./theme.json`         | your own file, found relative to the deck   |
| `https://…/theme.json` | a file fetched from a url                   |

- `headingColor` changes the headings without touching the rest
- a theme that fails to load falls back to the default, and the status bar says why

> uncomment `theme: kontrolplane` at the top of this file and save. the deck reloads in the new colours.

---

## on a big screen

```shell
lekture window presentation.md
lekture check presentation.md
```

- **window** opens the deck fullscreen in a window of its own, with the font sized so the screen is 100 columns wide. `--columns 90` makes the text bigger. it needs ghostty on macos
- **check** lists the slides that do not fit a terminal of 100 columns by 26 rows, and exits with status 1 when one does not, so it can run in ci
- a slide that is cut off while you present says so in the status bar

> a terminal cannot scale text. fewer columns is the only way to bigger letters.

---

## for people who are not in the room

```shell
lekture serve presentation.md
ssh localhost -p 53531
```

- **serve** presents the deck over ssh. everyone connected sees your edits when you save
- nobody has to log in, so served decks cannot run code unless you pass `--allow-exec`

```shell
lekture export presentation.md --output deck.html
lekture dump presentation.md | less -R
```

- **export** writes one html file with everything inside it. arrow keys move through it
- **dump** prints every slide to the terminal, for a pipe or a diff

---

# thank you

### lekture

- install it with `brew install kontrolplane/tap/lekture`
- the source is at `github.com/kontrolplane/lekture`
- two more decks to run: `example/kubernetes.md` and `example/helm.md`
