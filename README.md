<p align="center">
  <h1 align="center">
    <a href="https://kontrolplane.dev">
      <img width="1500" alt="kontrolplane header" src="./assets/kontrolplane-header.svg">
    </a>
  </h1>
</p>

`Lekture` is a terminal user interface (tui) application designed for presenting markdown-based slideshows. It provides an intuitive and efficient way to create and deliver presentations directly from the terminal. With Lekture, you can render markdown with full formatting, display inline images, execute code blocks, and even serve presentations over SSH, making it an essential tool for engineers who prefer working within a terminal environment.

## usage

```bash
lekture <presentation.md>
```

```bash
lekture serve <presentation.md> [--port PORT] [--host HOST]
```

```bash
cat presentation.md | lekture
```

## demonstration

`slide overview`
<p align="center">
  <img width="2400" alt="lekture slide overview" src="./assets/pages/slide/overview.png">
</p>

`code blocks with syntax highlighting`
<p align="center">
  <img width="2400" alt="lekture code block" src="./assets/pages/slide/code-block.png">
</p>

`code execution`
<p align="center">
  <img width="2400" alt="lekture code execution" src="./assets/pages/slide/code-execution.png">
</p>

## development

- [go](https://go.dev/) 1.26+

```bash
go build -o lekture . && ./lekture example/presentation.md
```

To show the changes made in the repository readme, the following command can be ran which automatically creates the preview gif & screenshots:

```bash
vhs vhs/cassette.tape
```

## keybindings

- `q`, `esc`, `ctrl+c`: quit
- `→`, `l`, `j`, `n`, `space`, `enter`: next slide
- `←`, `h`, `k`, `p`, `backspace`: previous slide
- `gg`, `home`: first slide
- `shift+g`, `end`: last slide
- `[n]j`: forward n slides
- `[n]shift+g`: jump to slide n
- `/`: search (supports regex, `/i` for case-insensitive)
- `ctrl+n`: next search result
- `ctrl+e`: execute code block

## contributors

[//]: kontrolplane/generate-contributors-list

<a href="https://github.com/levivannoort"><img src="https://avatars.githubusercontent.com/u/73097785?v=4" title="levivannoort" width="50" height="50"></a>

[//]: kontrolplane/generate-contributors-list

</br>

<p align="center">
  <img width="1500" alt="kontrolplane footer" src="./assets/kontrolplane-footer.svg">
</p>
