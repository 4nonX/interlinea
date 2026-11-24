package textfmt

import (
	"fmt"
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
			write: func(t string) string {
				// An empty line would end the cue: lines are closed up.
				var keep []string
				for _, l := range strings.Split(t, "\n") {
					if strings.TrimSpace(l) != "" {
						keep = append(keep, l)
					}
				}
				return strings.Join(keep, nl)
			}})
		i = last
	}
	if len(out) == 0 && !vtt {
		return nil, fmt.Errorf("the file has no subtitles: no line with times (00:00:01,000 --> 00:00:02,000)")
	}
	return out, nil
}
