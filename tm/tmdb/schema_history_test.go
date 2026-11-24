package tmdb

import (
	"testing"

	"github.com/4nonX/interlinea/internal/sqlstore/schemahistory"
)

// The steps of the schema stay as they were once databases have taken
// them; see the package schemahistory.
func TestSchemaHistory(t *testing.T) {
	schemahistory.Check(t, "testdata/schema-history.txt", map[string][]string{"sqlite": migrations, "postgres": pgMigrations})
}
