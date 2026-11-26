package tm

import (
	"testing"

	"github.com/4nonX/interlinea/internal/fuzzseed"
)

// FuzzReadFile reads memory files: TMX, .sdltm and .txt memories.
func FuzzReadFile(f *testing.F) {
	fuzzseed.Add(f, "internal/realworld/testdata/*/*.sdltm", "internal/realworld/testdata/wordfast/*.txt")
	f.Add([]byte(`<?xml version="1.0"?><tmx version="1.4"><header srclang="de-DE" segtype="sentence" datatype="plaintext" adminlang="en" creationtool="x" creationtoolversion="1" o-tmf="x"/><body><tu tuid="1" creationdate="20260101T120000Z"><prop type="x-client">Weber</prop><tuv xml:lang="de-DE"><seg>Der <bpt i="1">&lt;b&gt;</bpt>Mieter<ept i="1">&lt;/b&gt;</ept> zahlt.</seg></tuv><tuv xml:lang="en-GB"><seg>The <bpt i="1">&lt;b&gt;</bpt>tenant<ept i="1">&lt;/b&gt;</ept> pays.</seg></tuv></tu></body></tmx>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		ReadFile(data)
	})
}
