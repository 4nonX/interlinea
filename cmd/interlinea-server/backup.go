package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/4nonX/interlinea/internal/sqlitedb"
)

// storeFiles are the SQLite files of a server's data folder.
var storeFiles = []string{"accounts.db", "projects.db", "memories.db", "termbases.db"}

// backup copies the SQLite stores into a new folder while the server
// runs: each file is written with VACUUM INTO, a consistent copy of the
// database at one moment, without its write-ahead log. Restored, the copies
// are a complete data folder.
func backup(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("interlinea-server backup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	data := fs.String("data", env("INTERLINEA_DATA", "data"), "the server's data folder (INTERLINEA_DATA)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: interlinea-server backup [-data folder] new-folder")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Copies the server's SQLite files into new-folder, which must not exist")
		fmt.Fprintln(stderr, "yet. The server may keep running. On PostgreSQL, back up with pg_dump.")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("say which new folder the backup goes into")
	}
	if os.Getenv("INTERLINEA_POSTGRES") != "" {
		return errors.New("the server keeps its data in PostgreSQL (INTERLINEA_POSTGRES): back it up with pg_dump")
	}
	to, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("%s exists already: a backup goes into a new folder", to)
	}
	for _, name := range storeFiles {
		if _, err := os.Stat(filepath.Join(*data, name)); err != nil {
			return fmt.Errorf("%s is not a server's data folder: %w", *data, err)
		}
	}
	if err := os.MkdirAll(to, 0o700); err != nil {
		return err
	}
	for _, name := range storeFiles {
		src, err := filepath.Abs(filepath.Join(*data, name))
		if err != nil {
			return err
		}
		if err := vacuumInto(src, filepath.Join(to, name)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		st, err := os.Stat(filepath.Join(to, name))
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s: %d bytes\n", name, st.Size())
	}
	fmt.Fprintln(stdout, "backup written to", to)
	return nil
}

func vacuumInto(src, dst string) error {
	db, err := sql.Open("sqlite3", sqlitedb.FileDSN(filepath.ToSlash(src)))
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`VACUUM INTO ?`, dst); err != nil {
		return err
	}
	return os.Chmod(dst, 0o600)
}
