package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/4nonX/interlinea/align"
	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/tm"
)

func (s *Server) alignRoutes() {
	s.handle("POST /api/align", s.signedIn(s.alignFiles))
	s.handle("POST /api/memories/{name}/units/batch", s.signedIn(s.addUnits))
}

// alignedFile is one of the two documents of an alignment: its name, which
// tells the format, its language and its bytes in base64.
type alignedFile struct {
	Name string `json:"name"`
	Lang string `json:"lang"`
	Data []byte `json:"data"`
}

type alignRequest struct {
	Source alignedFile `json:"source"`
	Target alignedFile `json:"target"`
	// Fixed are pairs made or checked by hand, by the positions of their
	// segments in an earlier answer for the same documents: they stay, and
	// the rest is paired again around them.
	Fixed []align.Fixed `json:"fixed,omitempty"`
	// Order is the order the review reads each side in, by the positions
	// of the segments in the documents; a side not given keeps its
	// document's order. Fixed and the answer name segments by their
	// positions in the documents either way.
	Order align.Order `json:"order,omitempty"`
}

// alignFiles pairs the segments of a document with those of its
// translation, so that translations done before can be put into a memory
// (POST /api/memories/{name}/units/batch). The two documents may be of
// different formats. The answer has the segments of both documents and
// the pairs made of them; a review joins and splits them otherwise. Sent
// again with the pairs the review fixed, the rest is paired around them;
// sent with an order, where the review moved sections of one side, the
// segments are paired as read in that order.
// Nothing is kept.
//
// Access: signed in.
func (s *Server) alignFiles(w http.ResponseWriter, r *http.Request, me session) {
	var req alignRequest
	if !readJSON(w, r, &req, maxFileBody) {
		return
	}
	read := func(field string, f *alignedFile) ([]align.Segment, bool) {
		f.Name = strings.TrimSpace(f.Name)
		if f.Name == "" {
			invalid(w, field+".name", "Say the file's name, which tells its format.")
			return nil, false
		}
		lang, err := langtag.Normalize(f.Lang)
		if err != nil {
			invalid(w, field+".lang", "Say the language the document is in.")
			return nil, false
		}
		segs, err := align.Read(f.Name, f.Data, lang)
		if err != nil {
			invalid(w, field+".data", fmt.Sprintf("%s could not be read: %v.", f.Name, err))
			return nil, false
		}
		if len(segs) == 0 {
			invalid(w, field+".data", f.Name+" has no text.")
			return nil, false
		}
		return segs, true
	}
	src, ok := read("source", &req.Source)
	if !ok {
		return
	}
	tgt, ok := read("target", &req.Target)
	if !ok {
		return
	}
	if msg := align.CheckFixed(req.Fixed, len(src), len(tgt)); msg != "" {
		invalid(w, "fixed", msg)
		return
	}
	if msg := align.CheckOrder(req.Order, len(src), len(tgt)); msg != "" {
		invalid(w, "order", msg)
		return
	}
	writeJSON(w, http.StatusOK, align.DocumentsInOrder(src, tgt, req.Fixed, req.Order))
}

// maxUnitBatch is how many units one request may add.
const maxUnitBatch = 50000

// addUnits adds many units to a memory at once, all or none. ?policy= is
// as for one unit. The answer says how many were added.
//
// Access: edit on the memory.
func (s *Server) addUnits(w http.ResponseWriter, r *http.Request, me session) {
	m, _, ok := s.openMemoryFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	policy := tm.KeepBoth
	switch r.URL.Query().Get("policy") {
	case "", "keep":
	case "overwrite":
		policy = tm.Overwrite
	default:
		invalid(w, "policy", "The policy is keep (another translation of the same source is kept) or overwrite.")
		return
	}
	var req struct {
		Units []unitInput `json:"units"`
	}
	if !readJSON(w, r, &req, maxFileBody) {
		return
	}
	if len(req.Units) == 0 {
		invalid(w, "units", "There are no units to add.")
		return
	}
	if len(req.Units) > maxUnitBatch {
		invalid(w, "units", fmt.Sprintf("At most %d units can be added at once.", maxUnitBatch))
		return
	}
	units := make([]tm.Unit, len(req.Units))
	for i, u := range req.Units {
		if strings.TrimSpace(u.Source.Plain()) == "" {
			invalid(w, fmt.Sprintf("units[%d].source", i), "The source is empty.")
			return
		}
		if strings.TrimSpace(u.Target.Plain()) == "" {
			invalid(w, fmt.Sprintf("units[%d].target", i), "The translation is empty.")
			return
		}
		units[i] = u.unit(me.by())
	}
	if err := m.AddBatch(r.Context(), units, policy); err != nil {
		if !unitProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Added int `json:"added"`
	}{len(units)})
}
