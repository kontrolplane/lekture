package export

import "html/template"

// pageTemplate renders the whole deck as one self-contained document: no
// external stylesheets, scripts, fonts, or images, so the file can be shared
// or archived on its own.
var pageTemplate = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root {
  --accent: {{.HeadingColor}};
  --bg: #ffffff;
  --fg: #1e1e2e;
  --muted: #6b7280;
  --rule: #e5e7eb;
  --code-bg: #f6f7f9;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #14151a;
    --fg: #e6e6e6;
    --muted: #9aa0aa;
    --rule: #2b2d35;
    --code-bg: #1c1e26;
  }
}
* { box-sizing: border-box; }
html, body { margin: 0; padding: 0; }
body {
  background: var(--bg);
  color: var(--fg);
  font: 16px/1.65 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  padding: 0 1rem 6rem;
}
main { max-width: 52rem; margin: 0 auto; }
header.deck {
  padding: 3rem 0 1rem;
  border-bottom: 1px solid var(--rule);
  margin-bottom: 1rem;
}
header.deck h1 { font-size: 1.1rem; margin: 0; font-weight: 600; }
header.deck .meta { color: var(--muted); font-size: .9rem; margin-top: .25rem; }
section.slide {
  padding: 3rem 0;
  border-bottom: 1px solid var(--rule);
  scroll-margin-top: 1rem;
}
section.slide:last-of-type { border-bottom: 0; }
section.slide > :first-child { margin-top: 0; }
h1, h2, h3, h4, h5, h6 { color: var(--accent); line-height: 1.25; }
h1 { font-size: 2.25rem; }
h2 { font-size: 1.6rem; }
h3 { font-size: 1.25rem; }
a { color: var(--accent); }
img { max-width: 100%; height: auto; display: block; margin: 1.25rem 0; }
blockquote {
  margin: 1.25rem 0;
  padding: .25rem 0 .25rem 1rem;
  border-left: 3px solid var(--accent);
  color: var(--muted);
}
table { border-collapse: collapse; width: 100%; margin: 1.25rem 0; display: block; overflow-x: auto; }
th, td { border: 1px solid var(--rule); padding: .45rem .7rem; text-align: left; }
th { background: var(--code-bg); }
code {
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  font-size: .9em;
}
:not(pre) > code {
  background: var(--code-bg);
  padding: .15em .35em;
  border-radius: 4px;
}
pre {
  background: var(--code-bg);
  padding: 1rem;
  border-radius: 8px;
  overflow-x: auto;
  font-size: .9rem;
  line-height: 1.5;
}
pre code { background: none; padding: 0; }
.counter {
  position: fixed;
  right: 1rem;
  bottom: 1rem;
  background: var(--bg);
  border: 1px solid var(--rule);
  border-radius: 999px;
  padding: .3rem .8rem;
  font-size: .8rem;
  color: var(--muted);
}
@media print {
  .counter { display: none; }
  section.slide { page-break-after: always; border-bottom: 0; }
}
</style>
</head>
<body>
<main>
<header class="deck">
  <h1>{{.Title}}</h1>
  {{if or .Author .Date}}<div class="meta">{{.Author}}{{if and .Author .Date}} — {{end}}{{.Date}}</div>{{end}}
</header>
{{range .Slides}}
<section class="slide" id="slide-{{.Index}}">
{{.HTML}}
</section>
{{end}}
</main>
<div class="counter" id="counter">1 / {{.Total}}</div>
<script>
(function () {
  var slides = Array.prototype.slice.call(document.querySelectorAll("section.slide"));
  var counter = document.getElementById("counter");
  if (!slides.length) { return; }

  function currentIndex() {
    var best = 0, bestTop = Infinity;
    for (var i = 0; i < slides.length; i++) {
      var top = Math.abs(slides[i].getBoundingClientRect().top);
      if (top < bestTop) { bestTop = top; best = i; }
    }
    return best;
  }

  function update() {
    counter.textContent = (currentIndex() + 1) + " / " + slides.length;
  }

  function go(delta) {
    var next = Math.min(Math.max(currentIndex() + delta, 0), slides.length - 1);
    slides[next].scrollIntoView({ behavior: "smooth", block: "start" });
  }

  document.addEventListener("keydown", function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey) { return; }
    switch (e.key) {
      case "ArrowRight": case "ArrowDown": case " ": case "j": case "n":
        e.preventDefault(); go(1); break;
      case "ArrowLeft": case "ArrowUp": case "k": case "p":
        e.preventDefault(); go(-1); break;
      case "Home": case "g":
        e.preventDefault(); slides[0].scrollIntoView({ behavior: "smooth", block: "start" }); break;
      case "End": case "G":
        e.preventDefault(); slides[slides.length - 1].scrollIntoView({ behavior: "smooth", block: "start" }); break;
    }
  });

  document.addEventListener("scroll", update, { passive: true });
  update();
})();
</script>
</body>
</html>
`))
