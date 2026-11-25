package sfnt

// The glyph substitutions (GSUB) a joining script needs: single
// substitutions (a letter's initial, medial, final or isolated form) and
// ligatures (lam with alef), also behind extension lookups.

type gsub struct {
	r reader
	// script tag, then feature tags to lookup indices.
	features map[string]map[string][]int
	lookups  []int // offsets of the lookup tables
}

func parseGSUB(b []byte) *gsub {
	if len(b) < 10 {
		return nil
	}
	g := &gsub{r: reader{b: b}, features: map[string]map[string][]int{}}
	r := &g.r
	scriptList, featureList, lookupList := r.u16(4), r.u16(6), r.u16(8)
	type feat struct {
		tag     string
		lookups []int
	}
	var feats []feat
	nf := r.u16(featureList)
	for i := 0; i < nf; i++ {
		rec := featureList + 2 + 6*i
		if rec+6 > len(b) {
			break
		}
		tag := string(b[rec : rec+4])
		off := featureList + r.u16(rec+4)
		n := r.u16(off + 2)
		ls := make([]int, 0, n)
		for k := 0; k < n; k++ {
			ls = append(ls, r.u16(off+4+2*k))
		}
		feats = append(feats, feat{tag, ls})
	}
	addLangSys := func(script string, at int) {
		m := g.features[script]
		if m == nil {
			m = map[string][]int{}
			g.features[script] = m
		}
		n := r.u16(at + 4)
		for k := 0; k < n; k++ {
			fi := r.u16(at + 6 + 2*k)
			if fi < len(feats) {
				m[feats[fi].tag] = append(m[feats[fi].tag], feats[fi].lookups...)
			}
		}
	}
	ns := r.u16(scriptList)
	for i := 0; i < ns; i++ {
		rec := scriptList + 2 + 6*i
		if rec+6 > len(b) {
			break
		}
		tag := string(b[rec : rec+4])
		off := scriptList + r.u16(rec+4)
		if def := r.u16(off); def != 0 {
			addLangSys(tag, off+def)
		}
	}
	nl := r.u16(lookupList)
	for i := 0; i < nl; i++ {
		g.lookups = append(g.lookups, lookupList+r.u16(lookupList+2+2*i))
	}
	if r.bad {
		return nil
	}
	return g
}

func (g *gsub) lookupsFor(script, feature string) []int {
	if m, ok := g.features[script]; ok {
		if ls := m[feature]; len(ls) > 0 {
			return ls
		}
	}
	if m, ok := g.features["DFLT"]; ok {
		return m[feature]
	}
	return nil
}

// coverage gives the coverage index of a glyph, or -1.
func (g *gsub) coverage(at int, glyph uint16) int {
	r := &g.r
	switch r.u16(at) {
	case 1:
		n := r.u16(at + 2)
		lo, hi := 0, n-1
		for lo <= hi {
			mid := (lo + hi) / 2
			v := r.u16(at + 4 + 2*mid)
			switch {
			case v == int(glyph):
				return mid
			case v < int(glyph):
				lo = mid + 1
			default:
				hi = mid - 1
			}
		}
	case 2:
		n := r.u16(at + 2)
		for k := 0; k < n; k++ {
			rec := at + 4 + 6*k
			start, end, idx := r.u16(rec), r.u16(rec+2), r.u16(rec+4)
			if int(glyph) >= start && int(glyph) <= end {
				return idx + int(glyph) - start
			}
		}
	}
	return -1
}

// subtables gives the lookup's type and its subtables, through extensions.
func (g *gsub) subtables(lookup int) (int, []int) {
	r := &g.r
	typ := r.u16(lookup)
	n := r.u16(lookup + 4)
	var subs []int
	for k := 0; k < n; k++ {
		at := lookup + r.u16(lookup+6+2*k)
		if typ == 7 {
			ext := r.u16(at + 2)
			at += r.u32(at + 4)
			if k == 0 {
				typ = ext
			}
		}
		subs = append(subs, at)
	}
	return typ, subs
}

func (g *gsub) single(sub int, glyph uint16) (uint16, bool) {
	r := &g.r
	ci := g.coverage(sub+r.u16(sub+2), glyph)
	if ci < 0 {
		return glyph, false
	}
	switch r.u16(sub) {
	case 1:
		return uint16(int(glyph) + r.i16(sub+4)), true
	case 2:
		if ci < r.u16(sub+4) {
			return uint16(r.u16(sub + 6 + 2*ci)), true
		}
	}
	return glyph, false
}

// ligature tries the ligatures starting at glyphs[i]: the ligature glyph
// and how many glyphs it takes.
func (g *gsub) ligature(sub int, glyphs []uint16, i int) (uint16, int) {
	r := &g.r
	ci := g.coverage(sub+r.u16(sub+2), glyphs[i])
	if ci < 0 || ci >= r.u16(sub+4) {
		return 0, 0
	}
	set := sub + r.u16(sub+6+2*ci)
	n := r.u16(set)
	for k := 0; k < n; k++ {
		lig := set + r.u16(set+2+2*k)
		glyph, comps := r.u16(lig), r.u16(lig+2)
		if comps < 1 || i+comps > len(glyphs) {
			continue
		}
		match := true
		for c := 1; c < comps; c++ {
			if r.u16(lig+4+2*(c-1)) != int(glyphs[i+c]) {
				match = false
				break
			}
		}
		if match {
			return uint16(glyph), comps
		}
	}
	return 0, 0
}

func (g *gsub) apply(script, feature string, glyphs []uint16) ([]uint16, bool) {
	ls := g.lookupsFor(script, feature)
	if len(ls) == 0 {
		return glyphs, false
	}
	out := append([]uint16(nil), glyphs...)
	for _, li := range ls {
		if li >= len(g.lookups) {
			continue
		}
		typ, subs := g.subtables(g.lookups[li])
		switch typ {
		case 1:
			for i := range out {
				for _, s := range subs {
					if ng, ok := g.single(s, out[i]); ok {
						out[i] = ng
						break
					}
				}
			}
		case 4:
			var next []uint16
			for i := 0; i < len(out); {
				done := false
				for _, s := range subs {
					if lg, n := g.ligature(s, out, i); n > 0 {
						next = append(next, lg)
						i += n
						done = true
						break
					}
				}
				if !done {
					next = append(next, out[i])
					i++
				}
			}
			out = next
		}
	}
	return out, true
}
