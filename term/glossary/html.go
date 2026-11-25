package glossary

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/4nonX/interlinea/langtag"
)

func htmlRuns(rs []Run) string {
	var b strings.Builder
	for _, r := range rs {
		t := html.EscapeString(r.Text)
		var cls []string
		if r.Small {
			cls = append(cls, "s")
		}
		if r.Grey {
			cls = append(cls, "g")
		}
		attrs := ""
		if len(cls) > 0 {
			attrs = ` class="` + strings.Join(cls, " ") + `"`
		}
		if r.Lang != "" {
			attrs += ` lang="` + html.EscapeString(r.Lang) + `"`
			if langtag.RTL(r.Lang) {
				attrs += ` dir="rtl"`
			}
		}
		if r.Bold {
			t = "<b>" + t + "</b>"
		}
		if r.Italic {
			t = "<i>" + t + "</i>"
		}
		if r.Strike {
			t = "<s>" + t + "</s>"
		}
		if attrs != "" {
			t = "<span" + attrs + ">" + t + "</span>"
		}
		b.WriteString(t)
	}
	return b.String()
}

func htmlBlock(b Block, class string) string {
	attrs := ` class="` + class + `"`
	if b.Lang != "" {
		attrs += ` lang="` + html.EscapeString(b.Lang) + `"`
	}
	if b.RTL {
		attrs += ` dir="rtl"`
	}
	return "<p" + attrs + ">" + htmlRuns(b.Runs) + "</p>"
}

// HTML writes the glossary as one web page that needs nothing else: an
// index of letters, a search, and a layout for printing.
func HTML(g *Glossary, made time.Time) []byte {
	var b strings.Builder
	dir := ""
	if langtag.RTL(g.Sort) {
		dir = ` dir="rtl"`
	}
	page := "A4"
	if g.Options.Page == "letter" {
		page = "letter"
	}
	fmt.Fprintf(&b, `<!DOCTYPE html>
<html lang="%s"%s>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="generator" content="Interlinea">
<title>%s</title>
<style>
:root { color-scheme: light dark; --text: #1d2430; --muted: #5f6673; --line: #d5d9e0; --head: #eef0f4; --bg: #fff; }
@media (prefers-color-scheme: dark) { :root { --text: #e6e8ec; --muted: #a2a8b3; --line: #343a45; --head: #232833; --bg: #161a20; } }
* { box-sizing: border-box; }
body { margin: 0; background: var(--bg); color: var(--text); font: 15px/1.45 "Noto Sans", system-ui, -apple-system, "Segoe UI", sans-serif; }
header, nav, main { max-width: 1100px; margin: 0 auto; padding: 0 24px; }
header { padding-top: 32px; }
h1 { margin: 0 0 4px; font-size: 26px; }
.sub { margin: 0 0 16px; color: var(--muted); font-size: 13px; }
nav { position: sticky; top: 0; background: var(--bg); padding-top: 8px; padding-bottom: 8px; border-bottom: 1px solid var(--line); display: flex; flex-wrap: wrap; gap: 6px 12px; align-items: center; }
nav input { flex: 1 1 220px; max-width: 320px; padding: 6px 10px; border: 1px solid var(--line); border-radius: 6px; font: inherit; background: var(--bg); color: inherit; }
nav a { color: inherit; text-decoration: none; font-weight: 600; padding: 2px 4px; }
nav a:hover { text-decoration: underline; }
.dict { column-width: 28em; column-gap: 32px; padding-top: 12px; }
h2 { margin: 18px 0 6px; font-size: 20px; break-after: avoid; column-span: none; }
.entry { break-inside: avoid; margin: 0 0 8px; }
.entry p { margin: 0; padding-inline-start: 14px; text-indent: -14px; }
.entry p.more { padding-inline-start: 28px; }
.s { font-size: 12px; }
.g { color: var(--muted); }
table { width: 100%%; border-collapse: collapse; margin: 8px 0 16px; }
th, td { text-align: start; vertical-align: top; padding: 6px 8px; border-bottom: 1px solid var(--line); }
th { background: var(--head); position: sticky; top: 49px; }
td p { margin: 0 0 2px; }
.none { color: var(--muted); padding: 24px 0; }
footer { max-width: 1100px; margin: 24px auto; padding: 0 24px; color: var(--muted); font-size: 12px; }
@media print {
  @page { size: %s; margin: 18mm 16mm; }
  body { font-size: 9.5pt; }
  nav, .none { display: none; }
  .dict { column-count: 2; column-width: auto; }
  th { position: static; }
  thead { display: table-header-group; }
  tr { break-inside: avoid; }
}
</style>
</head>
<body>
<header><h1>%s</h1><p class="sub">%s</p>%s</header>
`, html.EscapeString(g.Sort), dir, html.EscapeString(g.Title), page, html.EscapeString(g.Title), html.EscapeString(g.subtitle(made)), legendHTML(g))

	b.WriteString(`<nav aria-label="Letters"><input type="search" id="find" placeholder="Find a term" aria-label="Find a term">`)
	for i, s := range g.Sections {
		if s.Letter != "" {
			fmt.Fprintf(&b, `<a href="#l%d">%s</a>`, i, html.EscapeString(s.Letter))
		}
	}
	b.WriteString("</nav>\n<main>\n")
	if g.Table() {
		hs := g.Headers()
		for i, s := range g.Sections {
			if s.Letter != "" {
				fmt.Fprintf(&b, `<h2 id="l%d">%s</h2>`, i, html.EscapeString(s.Letter))
			}
			b.WriteString("<table><thead><tr>")
			for c, h := range hs {
				d := ""
				if c < len(g.Languages) && langtag.RTL(g.Languages[c]) {
					d = ` dir="rtl"`
				}
				fmt.Fprintf(&b, `<th%s>%s</th>`, d, html.EscapeString(h))
			}
			b.WriteString("</tr></thead><tbody>\n")
			for _, a := range s.Articles {
				for _, e := range a.Entries {
					b.WriteString(`<tr class="entry">`)
					for _, cell := range g.Cells(e) {
						b.WriteString("<td>")
						for _, bl := range cell {
							b.WriteString(htmlBlock(bl, "t"))
						}
						b.WriteString("</td>")
					}
					b.WriteString("</tr>\n")
				}
			}
			b.WriteString("</tbody></table>\n")
		}
	} else {
		b.WriteString(`<div class="dict">` + "\n")
		for i, s := range g.Sections {
			if s.Letter != "" {
				fmt.Fprintf(&b, `<h2 id="l%d">%s</h2>`+"\n", i, html.EscapeString(s.Letter))
			}
			for _, a := range s.Articles {
				b.WriteString(`<div class="entry">`)
				for _, bl := range g.Blocks(a) {
					cls := "head"
					if bl.Indent > 0 {
						cls = "more"
					}
					b.WriteString(htmlBlock(bl, cls))
				}
				b.WriteString("</div>\n")
			}
		}
		b.WriteString("</div>\n")
	}
	b.WriteString(`<p class="none" id="none" hidden>No term is like that.</p>
</main>
<footer>Made with Interlinea.</footer>
<script>
(function () {
  var find = document.getElementById("find"), none = document.getElementById("none");
  var entries = Array.prototype.slice.call(document.querySelectorAll(".entry"));
  var heads = Array.prototype.slice.call(document.querySelectorAll("h2, table"));
  var fold = function (s) { return s.normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase(); };
  var texts = entries.map(function (e) { return fold(e.textContent); });
  find.addEventListener("input", function () {
    var q = fold(find.value.trim()), shown = 0;
    entries.forEach(function (e, i) { var on = !q || texts[i].indexOf(q) >= 0; e.hidden = !on; if (on) shown++; });
    heads.forEach(function (h) { h.hidden = !!q; });
    document.querySelectorAll("table").forEach(function (t) { t.hidden = q && !t.querySelector("tr.entry:not([hidden])"); });
    none.hidden = shown > 0;
  });
})();
</script>
</body>
</html>
`)
	return []byte(b.String())
}

func legendHTML(g *Glossary) string {
	if l := g.Legend(); l != "" {
		return `<p class="sub"><i>` + html.EscapeString(l) + `</i></p>`
	}
	return ""
}
