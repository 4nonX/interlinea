package spell

import (
	"os"
	"testing"
)

// BenchmarkSuggestGerman needs INTERLINEA_BENCH_DICT (base path of a
// German dictionary such as de_DE_frami).
func BenchmarkSuggestGerman(b *testing.B) {
	base := os.Getenv("INTERLINEA_BENCH_DICT")
	if base == "" {
		b.Skip("set INTERLINEA_BENCH_DICT")
	}
	aff, _ := os.ReadFile(base + ".aff")
	dic, _ := os.ReadFile(base + ".dic")
	d, err := Load(aff, dic)
	if err != nil {
		b.Fatal(err)
	}
	words := []string{"Abdankungsurunde", "Abschlagsir", "Mietvertrg", "Kauton", "Hauss", "Arbeitsvertrga"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, w := range words {
			d.Suggest(w, 5)
		}
	}
}
