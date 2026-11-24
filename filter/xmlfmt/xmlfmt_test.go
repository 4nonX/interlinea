package xmlfmt

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

func upper(ex *filter.Extraction) filter.Translations {
	tr := filter.Translations{}
	for _, u := range ex.Units {
		var c content.Content
		for _, r := range u.Source {
			if r.Tag != nil {
				c = c.AppendTag(content.Tag{Kind: r.Tag.Kind, ID: r.Tag.ID})
			} else {
				c = c.Append(strings.ToUpper(r.Text))
			}
		}
		tr[u.ID] = c
	}
	return tr
}

func texts(ex *filter.Extraction) []string {
	var out []string
	for _, u := range ex.Units {
		out = append(out, u.Source.Plain())
	}
	return out
}

func TestAnyXML(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?>
<!-- A manual -->
<manual version="2">
  <title>Safety &amp; care</title>
  <chapter id="c1">
    <para>Switch the device <b>off</b> first. Then pull the plug<br/>and wait.</para>
    <para translate="no">MODEL-X200</para>
    <note>Use <code translate="no">reset()</code> only when asked.</note>
    <code><![CDATA[if (a < b) run();]]></code>
    <empty/>
    <count>3</count>
  </chapter>
</manual>
`
	ex, err := Extract(XML, []byte(src), "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Safety & care", "Switch the device off first.", "Then pull the plugand wait.", "Use  only when asked.", "if (a < b) run();", "3"}
	if got := texts(ex); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("units %q", got)
	}
	if tags := ex.Units[1].Source.Tags(); len(tags) != 2 || tags[0].Type != "b" || tags[0].Kind != content.TagOpen {
		t.Errorf("inline element %+v", tags)
	}
	same, err := Merge(XML, []byte(src), "en-GB", "", filter.Translations{})
	if err != nil || string(same) != src {
		t.Fatalf("without translations the file changed: %v\n%s", err, same)
	}
	out, err := Merge(XML, []byte(src), "en-GB", "de-DE", upper(ex))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{
		"<!-- A manual -->\n<manual version=\"2\">\n  <title>SAFETY &amp; CARE</title>",
		"<para>SWITCH THE DEVICE <b>OFF</b> FIRST. THEN PULL THE PLUG<br/>AND WAIT.</para>",
		"<para translate=\"no\">MODEL-X200</para>",
		"<note>USE <code translate=\"no\">reset()</code> ONLY WHEN ASKED.</note>",
		"<code>IF (A &lt; B) RUN();</code>",
		"<empty/>",
	} {
		if !strings.Contains(string(out), w) {
			t.Errorf("translated file lacks %s:\n%s", w, out)
		}
	}
	if _, err := Extract(XML, out, "de-DE"); err != nil {
		t.Errorf("the translated file does not read: %v", err)
	}
	for _, bad := range []string{"<a><b></a>", "<a>", "plain text & more"} {
		if _, err := Extract(XML, []byte(bad), "en"); err == nil {
			t.Errorf("%q is read", bad)
		}
	}
}

func odf(t *testing.T, content, styles string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string, method uint16) {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, body)
	}
	add("mimetype", "application/vnd.oasis.opendocument.text", zip.Store)
	add("content.xml", content, zip.Deflate)
	if styles != "" {
		add("styles.xml", styles, zip.Deflate)
	}
	add("META-INF/manifest.xml", `<manifest/>`, zip.Deflate)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func unzip(t *testing.T, data []byte) (map[string]string, []string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	var order []string
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
		order = append(order, f.Name)
		if f.Name == "mimetype" && f.Method != zip.Store {
			t.Error("mimetype is packed")
		}
	}
	return out, order
}

func TestOpenDocument(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?><office:document-content xmlns:office="o" xmlns:text="t" xmlns:table="tb" xmlns:draw="d"><office:body><office:text>` +
		`<text:sequence-decls><text:sequence-decl text:name="Illustration"/></text:sequence-decls>` +
		`<text:h text:outline-level="2">Lease</text:h>` +
		`<text:p text:style-name="P1">The tenant pays <text:span text:style-name="T1">the rent</text:span>.<text:note text:id="ftn1" text:note-class="footnote"><text:note-citation>1</text:note-citation><text:note-body><text:p>Due on the third day.</text:p></text:note-body></text:note> It is due monthly.</text:p>` +
		`<table:table><table:table-row><table:table-cell office:value-type="string"><text:p>Deposit</text:p></table:table-cell><table:table-cell office:value-type="float" office:value="2400"><text:p>2,400.00</text:p></table:table-cell></table:table-row></table:table>` +
		`<text:p>Two<text:s text:c="2"/>spaces<text:line-break/>and a line.</text:p><text:p/>` +
		`</office:text></office:body></office:document-content>`
	styles := `<?xml version="1.0" encoding="UTF-8"?><office:document-styles xmlns:office="o" xmlns:text="t" xmlns:style="s"><office:master-styles><style:master-page style:name="Standard"><style:header><text:p>Draft of the lease</text:p></style:header></style:master-page></office:master-styles></office:document-styles>`
	data := odf(t, body, styles)
	ex, err := Extract(Text, data, "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Lease", "The tenant pays the rent.", " It is due monthly.", "Due on the third day.", "Deposit", "Twospacesand a line.", "Draft of the lease"}
	if got := texts(ex); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("units %q", got)
	}
	if ex.Blocks[0].Heading != 2 || ex.Blocks[0].File != "content.xml" || ex.Blocks[len(ex.Blocks)-1].Part != filter.PartHeader {
		t.Errorf("blocks %+v", ex.Blocks)
	}
	out, err := Merge(Text, data, "en-GB", "de-DE", upper(ex))
	if err != nil {
		t.Fatal(err)
	}
	files, order := unzip(t, out)
	if order[0] != "mimetype" || files["mimetype"] != "application/vnd.oasis.opendocument.text" || files["META-INF/manifest.xml"] != "<manifest/>" {
		t.Errorf("entries %v", order)
	}
	for _, w := range []string{
		`<text:h text:outline-level="2">LEASE</text:h>`,
		`THE TENANT PAYS <text:span text:style-name="T1">THE RENT</text:span>.<text:note text:id="ftn1" text:note-class="footnote"><text:note-citation>1</text:note-citation><text:note-body><text:p>DUE ON THE THIRD DAY.</text:p></text:note-body></text:note> IT IS DUE MONTHLY.</text:p>`,
		`<text:p>DEPOSIT</text:p>`, `<text:p>2,400.00</text:p>`,
		`<text:p>TWO<text:s text:c="2"/>SPACES<text:line-break/>AND A LINE.</text:p><text:p/>`,
		`<text:sequence-decl text:name="Illustration"/>`,
	} {
		if !strings.Contains(files["content.xml"], w) {
			t.Errorf("content.xml lacks %s:\n%s", w, files["content.xml"])
		}
	}
	if !strings.Contains(files["styles.xml"], "<text:p>DRAFT OF THE LEASE</text:p>") {
		t.Errorf("styles.xml: %s", files["styles.xml"])
	}
	// Without translations every part is as it was.
	same, err := Merge(Text, data, "en-GB", "", filter.Translations{})
	if err != nil {
		t.Fatal(err)
	}
	if files, _ := unzip(t, same); files["content.xml"] != body || files["styles.xml"] != styles {
		t.Error("without translations the parts changed")
	}
	if _, err := Extract(Text, []byte("not a zip"), "en"); err == nil {
		t.Error("something that is no archive is read")
	}
}
