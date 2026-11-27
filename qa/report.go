package qa

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/4nonX/interlinea/i18n"
)

// Report is the result of a QA run for export: the issues with the text
// of their segments, and counts by check.
type Report struct {
	Document   string        `json:"document"`
	SourceLang string        `json:"sourceLang"`
	TargetLang string        `json:"targetLang"`
	Created    time.Time     `json:"created"`
	Segments   int           `json:"segments"`
	Checked    int           `json:"checked"`
	Errors     int           `json:"errors"`
	Warnings   int           `json:"warnings"`
	ByKind     []KindCount   `json:"byKind"`
	Issues     []ReportIssue `json:"issues"`
}

// KindCount is the number of issues a check found.
type KindCount struct {
	Kind  Kind   `json:"kind"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ReportIssue is an issue with its segment's number and text.
type ReportIssue struct {
	Issue
	Number int    `json:"number"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// NewReport builds a report from the segments that were checked and the
// issues Check returned. Segment numbers count from 1 in segment order.
func NewReport(document string, segs []Segment, issues []Issue, o Options, now time.Time) Report {
	r := Report{Document: document, SourceLang: o.SourceLang, TargetLang: o.TargetLang, Created: now.UTC(), Segments: len(segs)}
	index := map[string]int{}
	for i, s := range segs {
		index[s.ID] = i
		if !s.Skip {
			r.Checked++
		}
	}
	counts := map[Kind]int{}
	for _, is := range issues {
		i, ok := index[is.Segment]
		ri := ReportIssue{Issue: is}
		if ok {
			ri.Number = i + 1
			ri.Source = segs[i].Source.Plain()
			ri.Target = segs[i].Target.Plain()
		}
		r.Issues = append(r.Issues, ri)
		counts[is.Kind]++
		if is.Severity == Error {
			r.Errors++
		} else {
			r.Warnings++
		}
	}
	for _, k := range Kinds {
		if counts[k.Kind] > 0 {
			r.ByKind = append(r.ByKind, KindCount{Kind: k.Kind, Name: k.Name, Count: counts[k.Kind]})
		}
	}
	if counts[Grammar] > 0 {
		r.ByKind = append(r.ByKind, KindCount{Kind: Grammar, Name: KindName(Grammar), Count: counts[Grammar]})
	}
	sort.SliceStable(r.Issues, func(i, j int) bool { return r.Issues[i].Number < r.Issues[j].Number })
	return r
}

// KindName is the readable name of a check.
func KindName(k Kind) string {
	if k == Grammar {
		return i18n.N("Grammar")
	}
	for _, x := range Kinds {
		if x.Kind == k {
			return x.Name
		}
	}
	return string(k)
}

// JSON encodes the report, indented.
func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// CSV encodes the issues as comma-separated values with a UTF-8 byte
// order mark, so Excel opens it with the right characters.
func (r Report) CSV() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&buf)
	rows := [][]string{{"Segment", "Severity", "Check", "Message", "Found", "Suggestions", "Source", "Translation"}}
	for _, is := range r.Issues {
		rows = append(rows, []string{
			fmt.Sprint(is.Number), string(is.Severity), KindName(is.Kind), is.Message, is.Found,
			strings.Join(is.Suggestions, ", "), is.Source, is.Target,
		})
	}
	if err := w.WriteAll(rows); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// HTML renders the report as a standalone page to open, print or send.
func (r Report) HTML() []byte {
	var b strings.Builder
	e := html.EscapeString
	title := "QA report"
	if r.Document != "" {
		title = "QA report: " + r.Document
	}
	fmt.Fprintf(&b, `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>%s</title>
<style>
body{font:14px/1.45 system-ui,sans-serif;color:#1d2230;margin:2rem;max-width:72rem}
h1{font-size:1.4rem;margin:0 0 .25rem}
p.meta{color:#5a6070;margin:0 0 1.5rem}
table{border-collapse:collapse;width:100%%;margin-bottom:2rem}
th,td{text-align:left;vertical-align:top;padding:.4rem .6rem;border-bottom:1px solid #d9dce3}
th{font-weight:600;background:#f1f2f5}
td.n{text-align:right;font-variant-numeric:tabular-nums}
.error{color:#a3201b;font-weight:600}
.warning{color:#7a5a00}
.text{white-space:pre-wrap}
@media print{body{margin:0}}
</style></head><body>
`, e(title))
	fmt.Fprintf(&b, "<h1>%s</h1>\n", e(title))
	langs := ""
	if r.SourceLang != "" || r.TargetLang != "" {
		langs = fmt.Sprintf("%s to %s, ", e(r.SourceLang), e(r.TargetLang))
	}
	fmt.Fprintf(&b, "<p class=\"meta\">%s%d of %d segments checked on %s. %d errors, %d warnings.</p>\n",
		langs, r.Checked, r.Segments, r.Created.Format("2 January 2006, 15:04 UTC"), r.Errors, r.Warnings)
	if len(r.Issues) == 0 {
		b.WriteString("<p>No issues found.</p>\n</body></html>\n")
		return []byte(b.String())
	}
	b.WriteString("<table><thead><tr><th>Check</th><th>Issues</th></tr></thead><tbody>\n")
	for _, k := range r.ByKind {
		fmt.Fprintf(&b, "<tr><td>%s</td><td class=\"n\">%d</td></tr>\n", e(k.Name), k.Count)
	}
	b.WriteString("</tbody></table>\n")
	b.WriteString("<table><thead><tr><th>Segment</th><th>Severity</th><th>Check</th><th>Message</th><th>Source</th><th>Translation</th></tr></thead><tbody>\n")
	for _, is := range r.Issues {
		msg := e(is.Message)
		if len(is.Suggestions) > 0 {
			msg += "<br>Suggestions: " + e(strings.Join(is.Suggestions, ", "))
		}
		sev := "Warning"
		if is.Severity == Error {
			sev = "Error"
		}
		fmt.Fprintf(&b, "<tr><td class=\"n\">%d</td><td class=\"%s\">%s</td><td>%s</td><td>%s</td><td class=\"text\">%s</td><td class=\"text\">%s</td></tr>\n",
			is.Number, is.Severity, sev, e(KindName(is.Kind)), msg, e(is.Source), e(is.Target))
	}
	b.WriteString("</tbody></table>\n</body></html>\n")
	return []byte(b.String())
}
