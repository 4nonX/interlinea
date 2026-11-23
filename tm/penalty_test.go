package tm

import (
	"context"
	"testing"

	"github.com/4nonX/interlinea/content"
)

func TestPenalized(t *testing.T) {
	ctx := context.Background()
	m, err := NewInMemory(Info{Name: "m", SourceLang: "de", TargetLang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Der Mieter zahlt die Miete.", "Der Mieter zahlt die Kaution."} {
		if _, err := m.Add(ctx, Unit{Source: content.Text(s), Target: content.Text("x")}, KeepBoth); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Penalized(m, -1); err == nil {
		t.Fatal("negative penalty accepted")
	}
	if _, err := Penalized(m, 101); err == nil {
		t.Fatal("penalty above 100 accepted")
	}
	if same, _ := Penalized(m, 0); same != Memory(m) || Penalty(same) != 0 {
		t.Fatal("penalty 0 should return the memory itself")
	}
	p, err := Penalized(m, 5)
	if err != nil {
		t.Fatal(err)
	}
	if Penalty(p) != 5 || p.Info().Name != "m" {
		t.Fatalf("penalty %d, info %+v", Penalty(p), p.Info())
	}
	plain, _ := m.Lookup(ctx, content.Text("Der Mieter zahlt die Miete."), LookupOptions{MinScore: 50})
	got, _ := p.Lookup(ctx, content.Text("Der Mieter zahlt die Miete."), LookupOptions{MinScore: 50})
	if len(plain) != len(got) || len(got) == 0 {
		t.Fatalf("plain %v, penalized %v", plain, got)
	}
	for i := range got {
		if got[i].Score != plain[i].Score-5 {
			t.Fatalf("match %d: %d, want %d", i, got[i].Score, plain[i].Score-5)
		}
	}
	if got[0].Score != 95 {
		t.Fatalf("exact match after a 5-point penalty = %d", got[0].Score)
	}
	// A minimum the penalized scores can't reach returns nothing; one just
	// reachable keeps only the matches that stay above it.
	if none, _ := p.Lookup(ctx, content.Text("Der Mieter zahlt die Miete."), LookupOptions{MinScore: 96}); len(none) != 0 {
		t.Fatalf("min 96 = %v", none)
	}
	if one, _ := p.Lookup(ctx, content.Text("Der Mieter zahlt die Miete."), LookupOptions{MinScore: 95}); len(one) != 1 {
		t.Fatalf("min 95 = %v", one)
	}
	full, _ := Penalized(m, 100)
	if none, _ := full.Lookup(ctx, content.Text("Der Mieter zahlt die Miete."), LookupOptions{MinScore: 1}); len(none) != 0 {
		t.Fatalf("penalty 100 = %v", none)
	}
}
