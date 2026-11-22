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
