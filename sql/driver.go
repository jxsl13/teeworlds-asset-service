package sql

import (
	"context"
	"fmt"

	"github.com/golang-migrate/migrate/v4/database"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

var (
	DefaultMigrationsTable = "schema_migrations"
	ErrNilConfig           = fmt.Errorf("no config")
)

type config struct {
	Context         context.Context
	MigrationsTable string
}

func withInstance(pool *pgxpool.Pool, cfg *config) (database.Driver, error) {
	if cfg == nil {
		return nil, ErrNilConfig
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	return pgxmigrate.WithInstance(sqlDB, &pgxmigrate.Config{
		MigrationsTable: cfg.MigrationsTable,
	})
}
