// Package schemahistory keeps the steps of a store's schema from changing
// once they exist.
//
// A database remembers how many steps it has taken, not what they were.
// If a step that databases have already taken is edited, those databases
// never get the edit, and from then on they differ from new ones: the
// next step may fail on them, or silently do something else. A change to
// the schema is therefore always a new step at the end.
package schemahistory

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update-schema-history", false, "record new schema steps in testdata/schema-history.txt")

// Check compares the steps of a store's schemas (by name: "sqlite",
// "postgres") with the ones recorded in file. A recorded step that
// changed or went away fails the test; a new step at the end fails it
// until it is recorded with -update-schema-history.
func Check(t *testing.T, file string, schemas map[string][]string) {
	t.Helper()
	recorded := map[string][]string{}
	if data, err := os.ReadFile(file); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			f := strings.Fields(line)
			if len(f) == 3 {
				recorded[f[0]] = append(recorded[f[0]], f[2])
			}
		}
	} else if !*update {
		t.Fatalf("%s: %v (record the steps with -update-schema-history)", file, err)
	}
	var out strings.Builder
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	// A fixed order, so the file does not change without a reason.
	for i := range names {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	for _, name := range names {
		steps := schemas[name]
		was := recorded[name]
		if len(steps) < len(was) {
			t.Errorf("%s: %d steps, but %d are recorded: a step was removed", name, len(steps), len(was))
		}
		for i, step := range steps {
			sum := sha256.Sum256([]byte(step))
			hash := hex.EncodeToString(sum[:8])
			fmt.Fprintf(&out, "%s %d %s\n", name, i+1, hash)
			switch {
			case i < len(was) && was[i] != hash:
				t.Errorf("%s: step %d changed. Databases that took it keep the old one: put the change in a new step at the end", name, i+1)
			case i >= len(was) && !*update:
				t.Errorf("%s: step %d is new. Record it with: go test -run SchemaHistory -update-schema-history", name, i+1)
			}
		}
	}
	if *update && !t.Failed() {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
