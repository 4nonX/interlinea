package term

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/internal/unzip"
)

const maxXLSXPart = 128 << 20

func zipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			if f.UncompressedSize64 > maxXLSXPart {
				return nil, fmt.Errorf("%s is too large", name)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, maxXLSXPart))
		}
	}
	return nil, fmt.Errorf("missing %s", name)
}

// readXLSX returns the rows of the first worksheet of an Excel workbook.
func readXLSX(data []byte) ([][]string, error) {
	zr, err := unzip.Open(data)
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	wb, err := zipFile(zr, "xl/workbook.xml")
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	var w struct {
		Sheets []struct {
			ID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(wb, &w); err != nil || len(w.Sheets) == 0 {
		return nil, fmt.Errorf("xlsx: no worksheet")
	}
	relsData, err := zipFile(zr, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	var rels struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(relsData, &rels); err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	sheetPath := ""
	for _, r := range rels.Rels {
		if r.ID == w.Sheets[0].ID {
			sheetPath = r.Target
			if strings.HasPrefix(sheetPath, "/") {
				sheetPath = strings.TrimPrefix(sheetPath, "/")
			} else {
				sheetPath = path.Join("xl", sheetPath)
			}
		}
	}
	if sheetPath == "" {
		return nil, fmt.Errorf("xlsx: first worksheet not found")
	}
	var shared []string
	if ss, err := zipFile(zr, "xl/sharedStrings.xml"); err == nil {
		_, err := eachElement(ss, map[string]bool{"si": true}, func(n *xnode) error {
			shared = append(shared, siText(n))
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("xlsx: shared strings: %w", err)
		}
	}
	sheet, err := zipFile(zr, sheetPath)
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	var rows [][]string
	_, err = eachElement(sheet, map[string]bool{"row": true}, func(n *xnode) error {
		var row []string
		next := 0
		for i := range n.Nodes {
			c := &n.Nodes[i]
			if c.XMLName.Local != "c" {
				continue
			}
			col := next
			if ref := c.attr("r"); ref != "" {
				col = columnIndex(ref)
			}
			next = col + 1
			v := ""
			switch c.attr("t") {
			case "s":
				if x := c.child("v"); x != nil {
					if k, err := strconv.Atoi(x.text()); err == nil && k >= 0 && k < len(shared) {
						v = shared[k]
					}
				}
			case "inlineStr":
				if is := c.child("is"); is != nil {
					v = siText(is)
				}
			default:
				if x := c.child("v"); x != nil {
					v = x.text()
				}
			}
			for len(row) < col {
				row = append(row, "")
			}
			row = append(row, v)
		}
		rowNum, _ := strconv.Atoi(n.attr("r"))
		for rowNum > 0 && len(rows) < rowNum-1 {
			rows = append(rows, nil)
		}
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	return rows, nil
}

// siText reads a shared string: plain <t> or rich text runs <r><t>.
func siText(n *xnode) string {
	var b strings.Builder
	for i := range n.Nodes {
		c := &n.Nodes[i]
		switch c.XMLName.Local {
		case "t":
			b.WriteString(c.Text)
		case "r":
			if t := c.child("t"); t != nil {
				b.WriteString(t.Text)
			}
		}
	}
	return b.String()
}

// columnIndex turns "C7" into 2.
func columnIndex(ref string) int {
	n := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		n = n*26 + int(r-'A'+1)
	}
	return n - 1
}

func columnName(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

// writeXLSX writes rows as a workbook with one sheet; the first row is a
// bold header, frozen so it stays visible.
func writeXLSX(sheetName string, rows [][]string) []byte {
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><sheetData>`)
	for r, row := range rows {
		sheet.WriteString(`<row r="` + strconv.Itoa(r+1) + `">`)
		for c, v := range row {
			if v == "" {
				continue
			}
			style := ""
			if r == 0 {
				style = ` s="1"`
			}
			sheet.WriteString(`<c r="` + columnName(c) + strconv.Itoa(r+1) + `" t="inlineStr"` + style + `><is><t xml:space="preserve">` + esc(xmlSafe(v)) + `</t></is></c>`)
		}
		sheet.WriteString("</row>")
	}
	sheet.WriteString("</sheetData></worksheet>")
	name := sheetName
	if name == "" {
		name = "Terms"
	}
	if len([]rune(name)) > 31 {
		name = string([]rune(name)[:31])
	}
	name = strings.NewReplacer("/", " ", "\\", " ", "?", " ", "*", " ", "[", "(", "]", ")", ":", " ").Replace(name)
	files := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="` + escAttr(name) + `" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		{"xl/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`},
		{"xl/worksheets/sheet1.xml", sheet.String()},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, _ := zw.Create(f.name)
		_, _ = w.Write([]byte(f.body))
	}
	_ = zw.Close()
	return buf.Bytes()
}

// xmlSafe drops characters XML 1.0 does not allow.
func xmlSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r != 0xFFFE && r != 0xFFFF {
			return r
		}
		return -1
	}, s)
}
