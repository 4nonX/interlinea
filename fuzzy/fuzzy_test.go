package fuzzy

import (
	"testing"

	"github.com/4nonX/interlinea/content"
)

func TestTokenize(t *testing.T) {
	toks := TokenizeText("Der Preis: 3,50 EUR, don't 日本語!")
	want := []struct {
		k Kind
		s string
	}{
		{Word, "Der"}, {Word, "Preis"}, {Punct, ":"}, {Number, "3,50"}, {Word, "EUR"},
		{Punct, ","}, {Word, "don't"}, {Word, "日"}, {Word, "本"}, {Word, "語"}, {Punct, "!"},
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens: %+v", len(toks), toks)
	}
	for i, w := range want {
		if toks[i].Kind != w.k || toks[i].Text != w.s {
			t.Errorf("token %d = %v %q, want %v %q", i, toks[i].Kind, toks[i].Text, w.k, w.s)
		}
	}
	ko := TokenizeText("계약은 시작합니다")
	if len(ko) != 2 {
		t.Errorf("Korean words = %+v", ko)
	}
}

func TestScoreOrdering(t *testing.T) {
	base := "Der Mieter haftet für alle Schäden an der Mietsache."
	exact := ScoreText(base, base)
	caseOnly := ScoreText(base, "der Mieter haftet für alle Schäden an der Mietsache.")
	punct := ScoreText(base, "Der Mieter haftet für alle Schäden an der Mietsache!")
	oneWord := ScoreText(base, "Der Vermieter haftet für alle Schäden an der Mietsache.")
	twoWords := ScoreText(base, "Der Vermieter haftet für keine Schäden an der Mietsache.")
	unrelated := ScoreText(base, "Die Kündigungsfrist beträgt drei Monate.")
	if exact != 100 {
		t.Fatalf("exact = %d", exact)
	}
	if !(caseOnly < 100 && caseOnly > punct && punct > oneWord && oneWord > twoWords && twoWords > unrelated) {
		t.Fatalf("ordering broken: case=%d punct=%d one=%d two=%d unrelated=%d",
			caseOnly, punct, oneWord, twoWords, unrelated)
	}
	if oneWord < 85 || oneWord > 95 {
		t.Errorf("one changed word of ten = %d, expected about 90", oneWord)
	}
	if unrelated > 30 {
		t.Errorf("unrelated = %d", unrelated)
	}
}

func TestScoreTags(t *testing.T) {
	plain := content.Text("Siehe Anlage 1.")
	var tagged content.Content
	tagged = tagged.Append("Siehe ")
	tagged = tagged.AppendTag(content.Tag{Kind: content.TagOpen, ID: "1", Type: "bold"})
	tagged = tagged.Append("Anlage 1")
	tagged = tagged.AppendTag(content.Tag{Kind: content.TagClose, ID: "1", Type: "bold"})
	tagged = tagged.Append(".")

	s := Score(plain, tagged)
	if s >= 100 || s < 70 {
		t.Fatalf("tag-only difference = %d", s)
	}
	// Different IDs with the same shape are an exact match.
	other := tagged.Clone()
	other[1].Tag.ID = "7"
	other[3].Tag.ID = "7"
	if got := Score(tagged, other); got != 100 {
		t.Fatalf("same shape, other ids = %d", got)
	}
}

func TestScoreEdgeCases(t *testing.T) {
	if ScoreText("", "") != 100 {
		t.Error("empty")
	}
	if ScoreText(" ", "  ") != 99 {
		t.Error("whitespace only")
	}
	if ScoreText("", "Hallo") != 0 {
		t.Error("empty vs word")
	}
	if a, b := ScoreText("a b c", "c b a"), ScoreText("c b a", "a b c"); a != b {
		t.Errorf("not symmetric: %d vs %d", a, b)
	}
}

func BenchmarkScore(b *testing.B) {
	x := "Der Mieter haftet für alle Schäden an der Mietsache, die durch ihn verursacht werden."
	y := "Der Vermieter haftet nicht für Schäden an der Mietsache, die durch Dritte verursacht werden."
	for i := 0; i < b.N; i++ {
		ScoreText(x, y)
	}
}
