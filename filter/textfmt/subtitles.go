package textfmt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// parseSubtitles reads SubRip (.srt) and WebVTT (.vtt) subtitles: each
// cue's text is one string, its lines kept apart by a tag. The numbers,
// the times, and in WebVTT the header, notes, styles and regions stay.
func parseSubtitles(src string, vtt bool) ([]*slot, error) {
	ls := linesOf(src)
	nl := newline(src)
	if vtt && (len(ls) == 0 || !strings.HasPrefix(ls[0].text, "WEBVTT")) {
		return nil, fmt.Errorf("the file does not start with WEBVTT")
	}
	var out []*slot
	for i := 0; i < len(ls); i++ {
		if !strings.Contains(ls[i].text, "-->") {
			continue
		}
		// The text is the lines after the times, up to an empty one.
		first := i + 1
		last := i
		for j := first; j < len(ls) && strings.TrimSpace(ls[j].text) != ""; j++ {
			last = j
		}
		if last < first {
			continue
		}
		var lines []string
		for j := first; j <= last; j++ {
			lines = append(lines, ls[j].text)
		}
		key := strings.TrimSpace(ls[i].text)
		out = append(out, &slot{start: ls[first].start, end: ls[last].end, id: "c" + strconv.Itoa(len(out)+1), text: strings.Join(lines, "\n"), key: key, markup: true,
			// An empty line would end the cue: lines are closed up.
			write: func(t string) string { return closedLines(t, nl) }})
		i = last
	}
	if len(out) == 0 && !vtt {
		return nil, fmt.Errorf("the file has no subtitles: no line with times (00:00:01,000 --> 00:00:02,000)")
	}
	return out, nil
}

// cueLines reads the lines of a cue's text from a line on, up to an empty
// one; last is the index of its last line, or before from where there is
// none.
func cueLines(ls []line, from int) (last int) {
	last = from - 1
	for j := from; j < len(ls) && strings.TrimSpace(ls[j].text) != ""; j++ {
		last = j
	}
	return last
}

// closedLines writes a cue's lines with the file's line ending, without
// empty ones, which would end the cue.
func closedLines(t, nl string) string {
	var keep []string
	for _, l := range strings.Split(t, "\n") {
		if strings.TrimSpace(l) != "" {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, nl)
}

// sbvTimes is the line of a YouTube subtitle's times: 0:00:01.000,0:00:03.500.
var sbvTimes = regexp.MustCompile(`^\s*\d+:\d{2}:\d{2}\.\d{1,3}\s*,\s*\d+:\d{2}:\d{2}\.\d{1,3}\s*$`)

// subViewerTimes is the line of a SubViewer 2 subtitle's times:
// 00:00:01.00,00:00:03.50.
var subViewerTimes = regexp.MustCompile(`^\s*\d{2}:\d{2}:\d{2}\.\d{2}\s*,\s*\d{2}:\d{2}:\d{2}\.\d{2}\s*$`)

// parseSBV reads YouTube's SubViewer subtitles (.sbv): a line of times,
// the text up to an empty line.
func parseSBV(src string) ([]*slot, error) {
	return parseTimedLines(src, sbvTimes, "")
}

// parseTimedLines reads subtitles written as a line of times and the
// text up to an empty line; br, if set, parts the lines of a subtitle
// written on one line ([br] in SubViewer 2).
func parseTimedLines(src string, times *regexp.Regexp, br string) ([]*slot, error) {
	ls := linesOf(src)
	nl := newline(src)
	var out []*slot
	for i := 0; i < len(ls); i++ {
		if !times.MatchString(ls[i].text) {
			continue
		}
		last := cueLines(ls, i+1)
		if last <= i {
			continue
		}
		var lines []string
		for j := i + 1; j <= last; j++ {
			lines = append(lines, ls[j].text)
		}
		from, to, _ := strings.Cut(strings.TrimSpace(ls[i].text), ",")
		text := strings.Join(lines, "\n")
		write := func(t string) string { return closedLines(t, nl) }
		if br != "" {
			text = strings.ReplaceAll(text, br, "\n")
			write = func(t string) string { return closedLines(strings.ReplaceAll(t, "\r", ""), br) }
		}
		out = append(out, &slot{start: ls[i+1].start, end: ls[last].end, id: "c" + strconv.Itoa(len(out)+1), text: text,
			key: strings.TrimSpace(from) + " --> " + strings.TrimSpace(to), markup: true, write: write})
		i = last
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the file has no subtitles: no line with times (0:00:01.000,0:00:02.000)")
	}
	return out, nil
}

// assTags finds the markup of an Advanced SubStation Alpha line: override
// blocks ({\i1}, {\pos(10,20)}) and hard spaces (\h).
var assTags = regexp.MustCompile(`\{[^{}\n]*\}|\\h`)

// assDrawing finds an override that turns drawing on ({\p1}): what
// follows is a shape, not text.
var assDrawing = regexp.MustCompile(`\{[^{}]*\\p[1-9]`)

// parseASS reads Advanced SubStation Alpha (.ass) and SubStation Alpha
// (.ssa) subtitles: the text of each Dialogue line of the [Events]
// section, its line breaks (\N, \n) kept apart by tags, its override
// blocks as tags. The script's other sections, comments, and lines that
// draw shapes stay as they are.
func parseASS(src string) ([]*slot, error) {
	ls := linesOf(src)
	inEvents, sawEvents := false, false
	// The fields of an event, by the section's Format line; Text is the last.
	fields := []string{"Layer", "Start", "End", "Style", "Name", "MarginL", "MarginR", "MarginV", "Effect", "Text"}
	var out []*slot
	for _, l := range ls {
		t := strings.TrimSpace(l.text)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			inEvents = strings.EqualFold(t, "[Events]")
			sawEvents = sawEvents || inEvents
			continue
		}
		if !inEvents {
			continue
		}
		name, rest, ok := strings.Cut(l.text, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "format":
			fields = fields[:0]
			for _, f := range strings.Split(rest, ",") {
				fields = append(fields, strings.TrimSpace(f))
			}
			continue
		case "dialogue":
		default:
			continue
		}
		if len(fields) == 0 {
			continue
		}
		// The text starts after as many commas as there are fields before it.
		at := l.start + len(name) + 1
		vals := make([]string, 0, len(fields))
		for k := 0; k < len(fields)-1; k++ {
			c := strings.IndexByte(src[at:l.end], ',')
			if c < 0 {
				return nil, fmt.Errorf("a Dialogue line has fewer fields than its Format line: %q", l.text)
			}
			vals = append(vals, strings.TrimSpace(src[at:at+c]))
			at += c + 1
		}
		text := src[at:l.end]
		if strings.TrimSpace(text) == "" || assDrawing.MatchString(text) {
			continue
		}
		var key string
		var notes []string
		for k, f := range fields[:len(fields)-1] {
			switch strings.ToLower(f) {
			case "start":
				key = vals[k] + key
			case "end":
				key += " --> " + vals[k]
			case "style":
				if vals[k] != "" {
					notes = append(notes, "Style: "+vals[k])
				}
			case "name", "actor":
				if vals[k] != "" {
					notes = append(notes, "Speaker: "+vals[k])
				}
			}
		}
		lines := strings.NewReplacer(`\N`, "\n", `\n`, "\n").Replace(text)
		out = append(out, &slot{start: at, end: l.end, id: "c" + strconv.Itoa(len(out)+1), text: lines, key: key, notes: notes, markup: true, tags: assTags,
			write: func(t string) string {
				// The text is one line of the file.
				t = strings.ReplaceAll(strings.ReplaceAll(t, "\r", ""), "\n", `\N`)
				return t
			}})
	}
	if !sawEvents {
		return nil, fmt.Errorf("the file is not a SubStation Alpha script: it has no [Events] section")
	}
	return out, nil
}

// microDVDTags finds MicroDVD's control codes: {y:i}, {C:$0000FF}, {f:Arial}.
var microDVDTags = regexp.MustCompile(`\{[A-Za-z]:[^{}\n]*\}`)

var microDVDLine = regexp.MustCompile(`^\{(\d+)\}\{(\d*)\}`)

// parseMicroDVD reads MicroDVD subtitles (.sub): one line per subtitle,
// {first frame}{last frame}, its lines parted by |. A first subtitle of
// frame 1 to 1 that holds a number gives the frames per second; it is
// left alone, and the frames are shown as times.
//
// A .sub file of SubViewer 2 (a line of times, 00:00:01.00,00:00:03.00,
// and the text with [br] between its lines) is read as such.
func parseMicroDVD(src string) ([]*slot, error) {
	ls := linesOf(src)
	for _, l := range ls {
		if t := strings.TrimSpace(l.text); t != "" {
			if !microDVDLine.MatchString(t) && (strings.HasPrefix(t, "[INFORMATION]") || subViewerTimes.MatchString(t)) {
				return parseTimedLines(src, subViewerTimes, "[br]")
			}
			break
		}
	}
	fps := 0.0
	var out []*slot
	seen := false
	for _, l := range ls {
		m := microDVDLine.FindStringSubmatchIndex(l.text)
		if m == nil {
			if strings.TrimSpace(l.text) != "" && !seen {
				return nil, fmt.Errorf("the file is not a MicroDVD subtitle file: a line does not start with {frame}{frame}")
			}
			continue
		}
		seen = true
		from, to := l.text[m[2]:m[3]], l.text[m[4]:m[5]]
		text := l.text[m[1]:]
		if from == "1" && to == "1" && len(out) == 0 {
			if v, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err == nil && v > 0 && v < 1000 {
				fps = v
				continue
			}
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		key := "frames " + from + " to " + to
		if fps > 0 {
			key = frameTime(from, fps) + " --> " + frameTime(to, fps)
		}
		out = append(out, &slot{start: l.start + m[1], end: l.end, id: "c" + strconv.Itoa(len(out)+1), text: strings.ReplaceAll(text, "|", "\n"), key: key, markup: true, tags: microDVDTags,
			write: func(t string) string {
				// The subtitle is one line of the file.
				return closedLines(strings.ReplaceAll(t, "\r", ""), "|")
			}})
	}
	if !seen {
		return nil, fmt.Errorf("the file is not a MicroDVD subtitle file: it has no line {frame}{frame}text")
	}
	return out, nil
}

// frameTime is the time of a frame, as SubRip writes times.
func frameTime(frame string, fps float64) string {
	n, err := strconv.Atoi(frame)
	if err != nil || frame == "" {
		return frame
	}
	ms := int64(float64(n) / fps * 1000)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}
