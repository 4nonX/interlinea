package project

import (
	"encoding/csv"
	"math"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/analysis"
)

// A reviewer marks what is wrong with a translation as issues: each has
// a kind and a weight. From them a file gets a score, the way a scored
// review (LQA) is done: penalty points by weight, set against the number
// of words reviewed.

// Issue is something a reviewer found wrong with a segment's translation.
type Issue struct {
	ID string `json:"id"`
	// Category is what kind of mistake it is, one of IssueCategories.
	Category string `json:"category"`
	// Severity is how much it weighs, one of IssueSeverities.
	Severity string `json:"severity"`
	// Note says what is wrong, if the reviewer wrote it down.
	Note string `json:"note,omitempty"`
	By   string `json:"by"`
	At   string `json:"at"`
}

// MaxIssueNote is how long an issue's note may be, in characters.
const MaxIssueNote = 1000

// IssueCategories are the kinds of issue, in the order they are offered.
var IssueCategories = []string{"accuracy", "fluency", "terminology", "style", "locale", "formatting", "other"}

// IssueSeverities are the weights of an issue, lightest first.
var IssueSeverities = []string{"minor", "major", "critical"}

// SeverityPoints is what an issue of a severity costs: 1 for a minor
// one, 5 for a major one, 25 for a critical one; 0 for anything else.
func SeverityPoints(severity string) int {
	switch severity {
	case "minor":
		return 1
	case "major":
		return 5
	case "critical":
		return 25
	}
	return 0
}

// KnownIssue reports whether the category and the severity are ones an
// issue can have.
func KnownIssue(category, severity string) bool {
	in := func(list []string, x string) bool {
		for _, v := range list {
			if v == x {
				return true
			}
		}
		return false
	}
	return in(IssueCategories, category) && in(IssueSeverities, severity)
}

// Quality is the score of segments from the issues marked on them.
type Quality struct {
	// Words is the number of source words of the segments.
	Words int `json:"words"`
	// Segments is how many segments there are, and Marked how many of
	// them have an issue.
	Segments int `json:"segments"`
	Marked   int `json:"marked"`
	// Issues counts all issues; Minor, Major and Critical by severity.
	Issues   int `json:"issues"`
	Minor    int `json:"minor"`
	Major    int `json:"major"`
	Critical int `json:"critical"`
	// Categories counts the issues of each kind that has any.
	Categories map[string]int `json:"categories"`
	// Penalty is the sum of the issues' points.
	Penalty int `json:"penalty"`
	// PerThousand is the penalty for every thousand words, and Score is
	// 100 less the penalty in percent of the words, never below 0; both
	// to one decimal place. Without words both are 0 and 100.
	PerThousand float64 `json:"perThousand"`
	Score       float64 `json:"score"`
}

// QualityOf scores segments by their issues.
func QualityOf(segs []Segment) Quality {
	q := Quality{Categories: map[string]int{}}
	for _, s := range segs {
		q.Segments++
		q.Words += analysis.Count(s.Source).Words
		if len(s.Issues) > 0 {
			q.Marked++
		}
		for _, is := range s.Issues {
			q.Issues++
			q.Categories[is.Category]++
			q.Penalty += SeverityPoints(is.Severity)
			switch is.Severity {
			case "minor":
				q.Minor++
			case "major":
				q.Major++
			case "critical":
				q.Critical++
			}
		}
	}
	return q.rated()
}

// Add is the score of both together.
func (q Quality) Add(o Quality) Quality {
	out := q
	out.Categories = map[string]int{}
	for k, v := range q.Categories {
		out.Categories[k] += v
	}
	for k, v := range o.Categories {
		out.Categories[k] += v
	}
	out.Words += o.Words
	out.Segments += o.Segments
	out.Marked += o.Marked
	out.Issues += o.Issues
	out.Minor += o.Minor
	out.Major += o.Major
	out.Critical += o.Critical
	out.Penalty += o.Penalty
	return out.rated()
}

func (q Quality) rated() Quality {
	q.PerThousand, q.Score = 0, 100
	if q.Words > 0 {
		q.PerThousand = math.Round(float64(q.Penalty)*10000/float64(q.Words)) / 10
		q.Score = math.Max(0, math.Round((100-float64(q.Penalty)*100/float64(q.Words))*10)/10)
	}
	return q
}

// ReviewedFile is a file in one target language, for the review report.
type ReviewedFile struct {
	Name     string    `json:"name"`
	Lang     string    `json:"lang"`
	Segments []Segment `json:"segments"`
}

// FileQuality is the score of a file in a target language.
type FileQuality struct {
	File string `json:"file"`
	Lang string `json:"lang"`
	Quality
}

// Review is the scored review of files: each file's score and the score
// of all together.
type Review struct {
	Files []FileQuality `json:"files"`
	Total Quality       `json:"total"`
}

// ReviewOf scores files.
func ReviewOf(files []ReviewedFile) Review {
	out := Review{Files: []FileQuality{}, Total: QualityOf(nil)}
	for _, f := range files {
		q := QualityOf(f.Segments)
		out.Files = append(out.Files, FileQuality{File: f.Name, Lang: f.Lang, Quality: q})
		out.Total = out.Total.Add(q)
	}
	return out
}

// ReviewCSV lists every issue of the files as a CSV table, with its
// segment's number, source and translation, for handing a review over.
// It starts with a byte order mark so spreadsheet programs read UTF-8.
func ReviewCSV(files []ReviewedFile) []byte {
	var b strings.Builder
	b.WriteString("\xEF\xBB\xBF")
	w := csv.NewWriter(&b)
	w.Write([]string{"File", "Language", "Segment", "Category", "Severity", "Points", "Note", "By", "At", "Source", "Translation"})
	for _, f := range files {
		for i, s := range f.Segments {
			for _, is := range s.Issues {
				w.Write([]string{f.Name, f.Lang, strconv.Itoa(i + 1), is.Category, is.Severity, strconv.Itoa(SeverityPoints(is.Severity)), is.Note, is.By, is.At, s.Source.Plain(), s.Target.Plain()})
			}
		}
	}
	w.Flush()
	return []byte(b.String())
}
