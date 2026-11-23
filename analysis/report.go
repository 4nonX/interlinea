package analysis

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/tm"
)

// ContextAttributes are the attributes to store with a memory unit so a
// later analysis can find context matches: the source segments before
// and after it.
func ContextAttributes(before, after content.Content) []tm.Attribute {
	return []tm.Attribute{
		{Name: AttrContextBefore, Value: contextKey(before)},
		{Name: AttrContextAfter, Value: contextKey(after)},
	}
}

// JSON encodes the report, indented.
func (r Report) JSON() ([]byte, error) { return json.MarshalIndent(r, "", "  ") }

func formatWeighted(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) }

// CSV encodes the report as comma-separated values with a UTF-8 byte
// order mark, so Excel opens it with the right characters: one block of
// rows per file, then the project total.
func (r Report) CSV() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&buf)
	head := []string{"File", "Band", "Segments", "Words", "Characters", "Tags", "Placeables"}
	weighted := r.Weights != nil
	if weighted {
		head = append(head, "Weight %", "Weighted words")
	}
	rows := [][]string{head}
	line := func(file string, row Row) []string {
		l := []string{file, row.Name, strconv.Itoa(row.Segments), strconv.Itoa(row.Words), strconv.Itoa(row.Characters), strconv.Itoa(row.Tags), strconv.Itoa(row.Placeables)}
		if weighted {
			wt := r.weightPercent(row.Band)
			l = append(l, formatWeighted(wt), formatWeighted(row.Weighted))
		}
		return l
	}
	total := func(file string, c Counts, wsum float64) []string {
		l := []string{file, "Total", strconv.Itoa(c.Segments), strconv.Itoa(c.Words), strconv.Itoa(c.Characters), strconv.Itoa(c.Tags), strconv.Itoa(c.Placeables)}
		if weighted {
			l = append(l, "", formatWeighted(wsum))
		}
		return l
	}
	for _, f := range r.Files {
		for _, row := range f.Rows {
			rows = append(rows, line(f.Name, row))
		}
		rows = append(rows, total(f.Name, f.Total, f.Weighted))
	}
	if len(r.Files) > 1 {
		for _, row := range r.Rows {
			rows = append(rows, line("All files", row))
		}
		rows = append(rows, total("All files", r.Total, r.Weighted))
	}
	if err := w.WriteAll(rows); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// HTML renders the report as a standalone page to open, print or send
// to a client.
func (r Report) HTML() []byte {
	var b strings.Builder
	e := html.EscapeString
	b.WriteString(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>Analysis</title>
<style>
body{font:14px/1.45 system-ui,sans-serif;color:#1d2230;margin:2rem;max-width:64rem}
h1{font-size:1.4rem;margin:0 0 .25rem}
h2{font-size:1.1rem;margin:2rem 0 .5rem}
p.meta{color:#5a6070;margin:0 0 1rem}
table{border-collapse:collapse;width:100%}
th,td{text-align:right;padding:.35rem .6rem;border-bottom:1px solid #d9dce3;font-variant-numeric:tabular-nums}
th:first-child,td:first-child{text-align:left}
th{font-weight:600;background:#f1f2f5}
tr.total td{font-weight:600;border-top:2px solid #1d2230}
@media print{body{margin:0}h2{break-after:avoid}}
</style></head><body>
<h1>Analysis</h1>
`)
	memories := "no translation memory (word count only)"
	if len(r.Memories) > 0 {
		memories = "against " + strings.Join(r.Memories, ", ")
	}
	rule := "the segment before"
	if r.Context == ContextBoth {
		rule = "the segments before and after"
	}
	fmt.Fprintf(&b, "<p class=\"meta\">%s to %s, %s. Context matches compare %s. Created %s.</p>\n",
		e(r.SourceLang), e(r.TargetLang), e(memories), rule, r.Created.Format("2 January 2006, 15:04 UTC"))
	weighted := r.Weights != nil
	table := func(rows []Row, total Counts, wsum float64) {
		b.WriteString("<table><thead><tr><th>Band</th><th>Segments</th><th>Words</th><th>Characters</th><th>Tags</th><th>Placeables</th>")
		if weighted {
			b.WriteString("<th>Weight</th><th>Weighted words</th>")
		}
		b.WriteString("</tr></thead><tbody>\n")
		for _, row := range rows {
			fmt.Fprintf(&b, "<tr><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td>", e(row.Name), row.Segments, row.Words, row.Characters, row.Tags, row.Placeables)
			if weighted {
				wt := r.weightPercent(row.Band)
				fmt.Fprintf(&b, "<td>%s%%</td><td>%s</td>", formatWeighted(wt), formatWeighted(row.Weighted))
			}
			b.WriteString("</tr>\n")
		}
		fmt.Fprintf(&b, "<tr class=\"total\"><td>Total</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td>", total.Segments, total.Words, total.Characters, total.Tags, total.Placeables)
		if weighted {
			fmt.Fprintf(&b, "<td></td><td>%s</td>", formatWeighted(wsum))
		}
		b.WriteString("</tr>\n</tbody></table>\n")
	}
	if len(r.Files) > 1 {
		b.WriteString("<h2>All files</h2>\n")
		table(r.Rows, r.Total, r.Weighted)
	}
	for _, f := range r.Files {
		fmt.Fprintf(&b, "<h2>%s</h2>\n", e(f.Name))
		table(f.Rows, f.Total, f.Weighted)
	}
	b.WriteString("</body></html>\n")
	return []byte(b.String())
}

// ParseWeights reads weights written as "context=0,repetition=10,100=30".
func ParseWeights(s string) (map[Band]float64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	out := map[Band]float64{}
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return nil, fmt.Errorf("analysis: weight %q is not band=percent", part)
		}
		b := Band(strings.TrimSpace(k))
		if BandName(b) == string(b) {
			var names []string
			for _, x := range Bands {
				names = append(names, string(x.Band))
			}
			return nil, fmt.Errorf("analysis: unknown band %q (bands: %s)", k, strings.Join(names, ", "))
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "%")), 64)
		if err != nil || f < 0 {
			return nil, fmt.Errorf("analysis: weight %q is not a percentage", v)
		}
		out[b] = f
	}
	return out, nil
}

func (r Report) weightPercent(b Band) float64 {
	if x, ok := r.Weights[b]; ok {
		return x
	}
	if b == Locked {
		return 0
	}
	return 100
}
