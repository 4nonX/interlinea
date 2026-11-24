// Package langtag normalizes and compares BCP 47 language tags as they
// appear in translation files (de-DE, en_GB, EN-us, zh-Hant-TW).
package langtag

import (
	"errors"
	"strings"
)

// ErrInvalid is returned for tags that are empty or malformed.
var ErrInvalid = errors.New("langtag: invalid language tag")

// Normalize returns the canonical casing of a tag: language lower case,
// script title case, region upper case, other subtags lower case.
// Underscores are accepted as separators.
func Normalize(tag string) (string, error) {
	tag = strings.TrimSpace(strings.ReplaceAll(tag, "_", "-"))
	if tag == "" {
		return "", ErrInvalid
	}
	parts := strings.Split(tag, "-")
	for i, p := range parts {
		if p == "" || len(p) > 8 || !isAlnum(p) {
			return "", ErrInvalid
		}
		lp := strings.ToLower(p)
		switch {
		case i == 0:
			if len(p) < 2 || len(p) > 8 || !isAlpha(p) {
				return "", ErrInvalid
			}
			parts[i] = lp
		case len(p) == 4 && isAlpha(p) && i == 1:
			parts[i] = strings.ToUpper(lp[:1]) + lp[1:]
		case len(p) == 2 && isAlpha(p) && i <= 2:
			parts[i] = strings.ToUpper(p)
		case len(p) == 3 && isDigit(p) && i <= 2:
			parts[i] = p
		default:
			parts[i] = lp
		}
	}
	return strings.Join(parts, "-"), nil
}

// MustNormalize is Normalize for tags known to be valid; it panics otherwise.
func MustNormalize(tag string) string {
	n, err := Normalize(tag)
	if err != nil {
		panic(err)
	}
	return n
}

// Primary returns the primary language subtag in lower case ("de" for "de-AT").
func Primary(tag string) string {
	tag = strings.ReplaceAll(strings.TrimSpace(tag), "_", "-")
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		tag = tag[:i]
	}
	return strings.ToLower(tag)
}

// Quality describes how well two tags match.
type Quality int

const (
	// NoMatch means different languages.
	NoMatch Quality = iota
	// PrimaryMatch means same language in the same script, with a
	// different or missing region. Simplified and Traditional Chinese, or
	// Serbian in Latin and in Cyrillic letters, do not match: text in one
	// is not usable as the other.
	PrimaryMatch
	// ExactMatch means equal after normalization.
	ExactMatch
)

// Match compares two tags.
func Match(a, b string) Quality {
	na, errA := Normalize(a)
	nb, errB := Normalize(b)
	if errA != nil || errB != nil {
		return NoMatch
	}
	if na == nb {
		return ExactMatch
	}
	if Primary(na) == Primary(nb) {
		if sa, sb := Script(na), Script(nb); sa != "" && sb != "" && sa != sb {
			return NoMatch
		}
		return PrimaryMatch
	}
	return NoMatch
}

func isAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i] | 0x20
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

func isDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9') && !((c|0x20) >= 'a' && (c|0x20) <= 'z') {
			return false
		}
	}
	return true
}

// MustNormalizeOr is Normalize, returning the tag as it is when it is
// not valid.
func MustNormalizeOr(tag string) string {
	if n, err := Normalize(tag); err == nil {
		return n
	}
	return tag
}
