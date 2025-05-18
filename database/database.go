package database

import (
	"context"
	"database/sql"
	"os"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

func New(ctx context.Context) (*sql.DB, error) {
	dbURL := os.Getenv("DATABASE_URL")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	goose.SetDialect("postgres")
	goose.SetBaseFS(nil)

	if err := goose.Up(db, "./database/migrations"); err != nil {
		return nil, err
	}

	return db, nil
}
