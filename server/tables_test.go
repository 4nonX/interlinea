package server

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
)

// The bilingual tables for review in Word come into a project as the
// bilingual files they are, whatever their ending says, and go back as
// they came with the translations in their target cells.
func TestReviewTables(t *testing.T) {
	read := func(name string) []byte {
		b, err := os.ReadFile("../filter/xliff/testdata/tables/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	te := newTeam(t)
	req := leaseRequest()
	req.Settings.TargetLangs = []string{"en-GB"}
	req.Settings.Memories = map[string][]project.MemorySetting{"en-GB": req.Settings.Memories["en-GB"]}
	req.Memories = req.Memories[:1]
	req.Files = []fileUpload{
		{Name: "Lieferschein.docx.review.docx", Role: project.Translatable, Data: read("lieferschein.docx.review.docx")},
		{Name: "Mietvertrag_de_en.rtf", Role: project.Translatable, Data: read("memoq-two-column.rtf")},
		{Name: "Mietvertrag_de_en.docx", Role: project.Translatable, Format: "xliff", Data: read("memoq-two-column.docx")},
		{Name: "Lieferschein.rtf", Role: project.Translatable, Data: read("dejavu-external-view.rtf")},
	}
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	for _, c := range []struct {
		file, segment, contentType string
		count                      int
		target                     content.Content
		check                      func(out []byte) bool
	}{
		{"Lieferschein.docx.review.docx", "8c9f2b9e-0000-4000-8000-000000000002/2", "wordprocessingml", 3, content.Text("Size: 30 centimetres"), func(out []byte) bool {
			doc := unzip(t, out, "word/document.xml")
			return strings.Contains(doc, ">Size: 30 centimetres</w:t>") && strings.Contains(doc, ">Translated</w:t>")
		}},
		{"Mietvertrag_de_en.rtf", "a1b2c3d4-0004-4000-8000-000000000004", "application/rtf", 4, content.Text("Oil heating"), func(out []byte) bool {
			return strings.Contains(string(out), `{\uc1 Oil heating}\cell }`) && strings.Contains(string(out), `{\uc1 Translator confirmed}\cell }`)
		}},
		{"Mietvertrag_de_en.docx", "a1b2c3d4-0001-4000-8000-000000000001", "wordprocessingml", 3, content.Text("The tenant pays the rent."), func(out []byte) bool {
			return strings.Contains(unzip(t, out, "word/document.xml"), ">The tenant pays the rent.</w:t>")
		}},
		{"Lieferschein.rtf", "0000003", "application/rtf", 3, content.Content{{Tag: &content.Tag{Kind: content.TagStandalone, ID: "00110"}}, {Text: " (page 2)"}}, func(out []byte) bool {
			return strings.Contains(string(out), `{\uc1 \{00110\} (page 2)}\cell }`)
		}},
	} {
		var segs []projectdb.StoredSegment
		te.call("GET", base+"/segments?file="+url.QueryEscape(c.file)+"&lang=en-GB", te.pm, nil, 200, &segs)
		if len(segs) != c.count {
			t.Fatalf("%s: %d segments, want %d", c.file, len(segs), c.count)
		}
		found := false
		for _, sg := range segs {
			if sg.File == nil || !sg.File.Fixed {
				t.Errorf("%s: segment %s may be split or joined", c.file, sg.ID)
			}
			if sg.ID == c.segment {
				found = true
				te.call("PUT", segPath(p.ID, c.file, "en-GB", sg.ID, sg.Version), te.pm, project.Edit{Target: c.target, Status: project.StatusConfirmed}, 200, nil)
			}
		}
		if !found {
			t.Fatalf("%s has no segment %s", c.file, c.segment)
		}
		r := te.download(base+"/files/translated?name="+url.QueryEscape(c.file)+"&lang=en-GB", te.pm)
		if !c.check(r.body) {
			t.Errorf("%s: the delivered file lacks the translation", c.file)
		}
		if ct := r.header.Get("Content-Type"); !strings.Contains(ct, c.contentType) {
			t.Errorf("%s is sent as %s", c.file, ct)
		}
		if cd := r.header.Get("Content-Disposition"); !strings.Contains(cd, c.file) || strings.Contains(cd, "(en-GB)") {
			t.Errorf("%s is sent as %q", c.file, cd)
		}
	}
	// What the files are, before a project is made of them.
	for name, file := range map[string]string{"Lieferschein.docx.review.docx": "lieferschein.docx.review.docx", "X.rtf": "memoq-two-column.rtf"} {
		var in inspection
		te.call("POST", "/api/files/inspect?name="+name, te.pm, read(file), 200, &in, "Content-Type", "application/octet-stream")
		if in.Format != "xliff" || in.Bilingual == nil || !in.Translatable {
			t.Errorf("%s inspected: %+v", name, in)
		}
	}
	// An RTF file that is no bilingual table is refused with a reason.
	var in inspection
	te.call("POST", "/api/files/inspect?name=Brief.rtf", te.pm, []byte(`{\rtf1\ansi Lieber Herr Weber,\par}`), 200, &in, "Content-Type", "application/octet-stream")
	if in.Problem == "" || !strings.Contains(in.Problem, "not a bilingual table") {
		t.Errorf("plain RTF inspected: %+v", in)
	}
}
