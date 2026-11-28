// Package plaintext reads and writes plain text files: each line is a
// paragraph, cut into sentences by the source language's rules. Blank
// lines, the line ends and the spaces between sentences stay as they are.
// The text is UTF-8, or UTF-16 with a byte order mark, and is written
// back the way it came.
package plaintext

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/segment"
)

// Format is the name of the format.
const Format = "txt"

// Decode reads a text file: UTF-8, or UTF-16 with a byte order mark. bom
// is the mark the file started with, to write it back.
func Decode(data []byte) (text string, bom []byte, err error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		bom, data = data[:3], data[3:]
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		le := data[0] == 0xFF
		body := data[2:]
		if len(body)%2 != 0 {
			return "", nil, fmt.Errorf("the UTF-16 text has an odd number of bytes")
		}
		u := make([]uint16, len(body)/2)
		for i := range u {
			if le {
				u[i] = uint16(body[2*i]) | uint16(body[2*i+1])<<8
			} else {
				u[i] = uint16(body[2*i])<<8 | uint16(body[2*i+1])
			}
		}
		return string(utf16.Decode(u)), data[:2], nil
	}
	if !utf8.Valid(data) {
		return "", nil, fmt.Errorf("the text is not UTF-8 or UTF-16")
	}
	return string(data), bom, nil
}

// Encode writes text as the file was: UTF-8, or UTF-16 with its mark.
func Encode(text string, bom []byte) []byte {
	if bytes.Equal(bom, []byte{0xFF, 0xFE}) || bytes.Equal(bom, []byte{0xFE, 0xFF}) {
		out := append([]byte{}, bom...)
		for _, c := range utf16.Encode([]rune(text)) {
			if bom[0] == 0xFF {
				out = append(out, byte(c), byte(c>>8))
			} else {
				out = append(out, byte(c>>8), byte(c))
			}
		}
		return out
	}
	return append(append([]byte{}, bom...), text...)
}

// span is a sentence in the text.
type span struct {
	block      string
	start, end int
}

// spans finds the sentences: line by line, by the language's rules.
func spans(text, lang string) []span {
	seg := segment.New(nil, lang)
	var out []span
	para, at := 0, 0
	for _, line := range strings.SplitAfter(text, "\n") {
		body := strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(body) != "" {
			para++
			for _, sp := range seg.Split(body) {
				out = append(out, span{fmt.Sprintf("p%d", para), at + sp.Start, at + sp.End})
			}
		}
		at += len(line)
	}
	return out
}

// Extract returns the sentences of a text file as units. lang is the
// source language; it selects the sentence rules.
func Extract(data []byte, lang string) (*filter.Extraction, error) {
	text, _, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("plaintext: %w", err)
	}
	ex := &filter.Extraction{Format: Format}
	last := ""
	for i, sp := range spans(text, lang) {
		if sp.block != last {
			ex.Blocks = append(ex.Blocks, filter.Block{ID: sp.block, Part: filter.PartBody})
			last = sp.block
		}
		ex.Units = append(ex.Units, filter.Unit{ID: fmt.Sprintf("s%d", i+1), Block: sp.block, Source: content.Text(text[sp.start:sp.end])})
	}
	return ex, nil
}

// Merge returns the text with its sentences replaced by their
// translations. Sentences without one stay; sentences joined to the one
// before them are written with it, as one translation. Where that one is
// in an earlier paragraph, they are left out of their own, and a
// paragraph left with no sentence goes, with the blank line before it.
// lang must be the language used for Extract.
func Merge(data []byte, lang string, tr filter.Translations) ([]byte, error) {
	text, bom, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("plaintext: %w", err)
	}
	all := spans(text, lang)
	var b strings.Builder
	at := 0
	for i := 0; i < len(all); i++ {
		target, ok := tr[fmt.Sprintf("s%d", i+1)]
		if filter.IsJoined(target) {
			// Sentences that start their paragraph joined to one in an
			// earlier paragraph have their translation there.
			j := i
			for j+1 < len(all) && all[j+1].block == all[i].block && filter.IsJoined(tr[fmt.Sprintf("s%d", j+2)]) {
				j++
			}
			if i > 0 && (j+1 >= len(all) || all[j+1].block != all[i].block) {
				// The whole paragraph goes, with what parted it from the one before.
				b.WriteString(text[at:all[i-1].end])
				at = all[j].end
			} else {
				b.WriteString(text[at:all[i].start])
				at = all[j].end
				if j+1 < len(all) {
					at = all[j+1].start
				}
			}
			i = j
			continue
		}
		if !ok || len(target) == 0 {
			continue
		}
		start, end := all[i].start, all[i].end
		for i+1 < len(all) && all[i+1].block == all[i].block && filter.IsJoined(tr[fmt.Sprintf("s%d", i+2)]) {
			i++
			end = all[i].end
		}
		b.WriteString(text[at:start])
		b.WriteString(target.Plain())
		at = end
	}
	b.WriteString(text[at:])
	return Encode(b.String(), bom), nil
}
