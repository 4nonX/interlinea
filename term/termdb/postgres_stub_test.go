//go:build !postgres

package termdb

import "testing"

// Without the postgres build tag the tests run on SQLite only.

func pgDSN() string { return "" }

func openPG(t *testing.T) (*DB, string) {
	t.Fatal("built without the postgres tag")
	return nil, ""
}

func reopenPG(t *testing.T, dsn string) *DB {
	t.Fatal("built without the postgres tag")
	return nil
}

func isPG(string) bool { return false }
