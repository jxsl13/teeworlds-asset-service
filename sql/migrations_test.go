package sql_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	postgresql "github.com/jxsl13/teeworlds-asset-service/sql"
)

// devEnv reads key=value pairs from docker/dev.env relative to the module root
// and returns them as a map. Lines starting with # and empty lines are ignored.
func devEnv(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open("../docker/dev.env")
	if err != nil {
		t.Fatalf("open dev.env: %v", err)
	}
	defer f.Close()
	m := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, _ := strings.Cut(line, "=")
		m[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read dev.env: %v", err)
	}
	return m
}

func dsnFromEnv(t *testing.T) string {
	t.Helper()
	env := devEnv(t)
	return fmt.Sprintf(
		"postgres://%s:%s@localhost:%s/%s?sslmode=disable",
		env["POSTGRES_USER"], env["POSTGRES_PASSWORD"],
		env["POSTGRES_PORT"], env["POSTGRES_DB"],
	)
}

func connectPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := dsnFromEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping: could not create pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping: DB not reachable at %s: %v", dsn, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestMigrate(t *testing.T) {
	pool := connectPool(t)
	ctx := context.Background()
	if err := postgresql.Migrate(ctx, pool); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := postgresql.Migrate(ctx, pool); err != nil {
		t.Fatalf("idempotent Migrate: %v", err)
	}
}

func TestSearch(t *testing.T) {
	pool := connectPool(t)
	ctx := context.Background()
	if err := postgresql.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, "TRUNCATE search_value, asset_item, asset_group CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	groupID := uuid.New()
	itemID := uuid.New()
	if _, err := db.ExecContext(ctx,
		"INSERT INTO asset_group (group_id, asset_type, group_name, group_key) VALUES ($1, $2, $3, $4)",
		groupID, postgresql.AssetTypeEnumMap, "DDNet", "resolution",
	); err != nil {
		t.Fatalf("insert asset_group: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO asset_item (item_id, group_id, item_file_path, checksum) VALUES ($1, $2, $3, $4)",
		itemID, groupID, "/map/test.map", "abc123",
	); err != nil {
		t.Fatalf("insert asset_item: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO search_value (group_id, key_name, key_value) VALUES ($1, $2, $3)",
		groupID, "name", "DDnet",
	); err != nil {
		t.Fatalf("insert search_value: %v", err)
	}
	q := postgresql.New(db)
	t.Run("exact match returns item", func(t *testing.T) {
		results, err := q.Search(ctx, postgresql.SearchParams{
			StrictWordSimilarity: "DDNet",
			Limit:                10,
			Offset:               0,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected at least one result, got none")
		}
		if results[0].GroupID != groupID {
			t.Errorf("group_id: want %s, got %s", groupID, results[0].GroupID)
		}
		if results[0].AssetType != postgresql.AssetTypeEnumMap {
			t.Errorf("asset_type: want %s, got %s", postgresql.AssetTypeEnumMap, results[0].AssetType)
		}
		if results[0].Sml <= 0 {
			t.Errorf("sml score should be > 0, got %f", results[0].Sml)
		}
		if results[0].TotalCount != 1 {
			t.Errorf("total_count: want 1, got %d", results[0].TotalCount)
		}
	})
	t.Run("no match returns empty", func(t *testing.T) {
		results, err := q.Search(ctx, postgresql.SearchParams{
			StrictWordSimilarity: "xyzzy_no_match_ever",
			Limit:                10,
			Offset:               0,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected no results, got %d", len(results))
		}
	})
}
