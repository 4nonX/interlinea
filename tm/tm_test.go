package tm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/4nonX/interlinea/content"
)

var ctx = context.Background()

func newMem(t *testing.T) *InMemory {
	t.Helper()
	m, err := NewInMemory(Info{Name: "Recht", SourceLang: "de_de", TargetLang: "EN-gb"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func add(t *testing.T, m Memory, src, tgt string) Unit {
	t.Helper()
	u, err := m.Add(ctx, Unit{Source: content.Text(src), Target: content.Text(tgt), CreatedBy: "dan"}, KeepBoth)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestInfoValidation(t *testing.T) {
	m := newMem(t)
	if got := m.Info(); got.SourceLang != "de-DE" || got.TargetLang != "en-GB" {
		t.Fatalf("languages not normalized: %+v", got)
	}
	for _, bad := range []Info{{}, {Name: "x", SourceLang: "de"}, {Name: " ", SourceLang: "de", TargetLang: "en"}} {
		if _, err := NewInMemory(bad); !errors.Is(err, ErrInvalidInfo) {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestConcurrentUse(t *testing.T) {
	m := newMem(t)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				src := fmt.Sprintf("Satz %d aus Routine %d.", i, g)
				if _, err := m.Add(ctx, Unit{Source: content.Text(src), Target: content.Text("t")}, KeepBoth); err != nil {
					t.Error(err)
					return
				}
				if _, err := m.Lookup(ctx, content.Text(src), LookupOptions{}); err != nil {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	if n, _ := m.Count(ctx); n != 400 {
		t.Fatalf("count = %d", n)
	}
}

func BenchmarkLookup10k(b *testing.B) {
	m, _ := NewInMemory(Info{Name: "b", SourceLang: "de", TargetLang: "en"})
	words := strings.Fields("Mieter Vermieter haftet Schäden Mietsache Vertrag Kündigung Frist Monate Kaution Zahlung Miete Wohnung Nebenkosten Abrechnung jährlich schriftlich")
	for i := 0; i < 10000; i++ {
		var sb strings.Builder
		for j := 0; j < 10; j++ {
			sb.WriteString(words[(i*7+j*3)%len(words)])
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%d.", i)
		m.Add(ctx, Unit{Source: content.Text(sb.String()), Target: content.Text("x")}, KeepBoth)
	}
	q := content.Text("Der Mieter haftet für Schäden an der Mietsache laut Vertrag 42.")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Lookup(ctx, q, LookupOptions{})
	}
}

func TestInMemoryChangeAndDelete(t *testing.T) {
	m, err := NewInMemory(Info{Name: "Recht", SourceLang: "de-DE", TargetLang: "en-GB"})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := m.Add(ctx, Unit{Source: content.Text("Die Kaution ist fällig."), Target: content.Text("The deposit is due."), CreatedBy: "dan"}, KeepBoth)
	b, _ := m.Add(ctx, Unit{Source: content.Text("Der Mieter zahlt die Miete."), Target: content.Text("The tenant pays the rent."), CreatedBy: "dan"}, KeepBoth)
	c, _ := m.Add(ctx, Unit{Source: content.Text("Die Miete steigt."), Target: content.Text("The rent rises."), CreatedBy: "dan"}, KeepBoth)

	// A translation is changed; who made the unit stays.
	got, err := m.SetTarget(ctx, b.ID, content.Text("The tenant shall pay the rent."), "eva")
	if err != nil || got.Target.Plain() != "The tenant shall pay the rent." || got.CreatedBy != "dan" || got.ChangedBy != "eva" {
		t.Fatalf("changed: %+v %v", got, err)
	}
	if _, err := m.SetTarget(ctx, b.ID, content.Content{}, "eva"); !errors.Is(err, ErrEmptyTarget) {
		t.Fatalf("an empty translation: %v", err)
	}
	if _, err := m.SetTarget(ctx, 999, content.Text("x"), "eva"); !errors.Is(err, ErrNoUnit) {
		t.Fatalf("a unit that is not there: %v", err)
	}
	if ms, _ := m.Lookup(ctx, content.Text("Der Mieter zahlt die Miete."), LookupOptions{MinScore: 100}); len(ms) != 1 || ms[0].Unit.Target.Plain() != "The tenant shall pay the rent." {
		t.Fatalf("lookup after the change: %+v", ms)
	}

	// A unit is deleted; the others are still found, by match and by search.
	if err := m.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(ctx, a.ID); !errors.Is(err, ErrNoUnit) {
		t.Fatalf("deleted twice: %v", err)
	}
	if n, _ := m.Count(ctx); n != 2 {
		t.Fatalf("count %d", n)
	}
	if ms, _ := m.Lookup(ctx, content.Text("Die Kaution ist fällig."), LookupOptions{MinScore: 100}); len(ms) != 0 {
		t.Fatalf("the deleted unit still matches: %+v", ms)
	}
	if ms, _ := m.Lookup(ctx, content.Text("Die Miete steigt."), LookupOptions{MinScore: 100}); len(ms) != 1 || ms[0].Unit.ID != c.ID {
		t.Fatalf("lookup after the deletion: %+v", ms)
	}
	if us, _ := m.Concordance(ctx, "Miete", ConcordanceOptions{}); len(us) != 2 {
		t.Fatalf("search after the deletion: %+v", us)
	}
	// A new unit does not take the deleted one's ID.
	if d, _ := m.Add(ctx, Unit{Source: content.Text("Neu."), Target: content.Text("New.")}, KeepBoth); d.ID <= c.ID {
		t.Fatalf("ID of a new unit: %d", d.ID)
	}
}
