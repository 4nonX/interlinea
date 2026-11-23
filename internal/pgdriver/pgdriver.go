// Package pgdriver opens PostgreSQL databases for Interlinea's server
// stores with the pgx driver.
package pgdriver

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
)

// Open connects to the database named by a PostgreSQL connection string
// ("postgres://user:password@host:5432/interlinea?sslmode=disable" or
// "host=... dbname=...") and checks that it answers.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("pgdriver: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pgdriver: connect: %w", err)
	}
	return db, nil
}
