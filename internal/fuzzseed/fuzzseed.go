// Package fuzzseed gives the fuzz tests of the readers their first
// inputs: the sample files of the repository. Every `go test` runs a fuzz
// test on these inputs; `go test -fuzz` goes on from them with inputs of
// its own.
package fuzzseed

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// MaxSize is the largest sample file used: larger ones slow every run and
// add little the fuzzer would not find from the smaller ones.
const MaxSize = 1 << 20

// Add adds the files matching the patterns, relative to the repository's
// root ("filter/xliff/testdata/*.xlf"), as inputs of f, for a fuzz test
// whose only input is the file. It fails the test when no file matches
// at all, so a moved folder is noticed.
func Add(f *testing.F, patterns ...string) {
	f.Helper()
	for _, data := range Files(f, patterns...) {
		f.Add(data)
	}
}

// Files returns the files Add would add, for a fuzz test that takes more
// inputs than the file.
func Files(f *testing.F, patterns ...string) [][]byte {
	f.Helper()
	root, err := Root()
	if err != nil {
		f.Fatal(err)
	}
	var files []string
	for _, p := range patterns {
		m, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			f.Fatal(err)
		}
		files = append(files, m...)
	}
	sort.Strings(files)
	var out [][]byte
	for _, name := range files {
		st, err := os.Stat(name)
		if err != nil || st.IsDir() || st.Size() > MaxSize {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}
		out = append(out, data)
	}
	if len(out) == 0 {
		f.Fatalf("no sample file matches %q", patterns)
	}
	return out
}

// Root is the repository's root: the nearest folder above the test's own
// with a go.mod.
func Root() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		up := filepath.Dir(dir)
		if up == dir {
			return "", os.ErrNotExist
		}
		dir = up
	}
}
