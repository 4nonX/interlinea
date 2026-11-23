//go:build !postgres

package projectdb

import "testing"

// Without the postgres build tag the tests run on SQLite only.

func pgDSN() string { return "" }

func openPG(t testing.TB) (*DB, string) {
	t.Fatal("built without the postgres tag")
	return nil, ""
}

func reopenPG(t testing.TB, dsn string) *DB {
	t.Fatal("built without the postgres tag")
	return nil
}

func isPG(string) bool { return false }
