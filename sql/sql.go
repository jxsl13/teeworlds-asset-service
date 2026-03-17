package sql

import (
	"context"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

//go:generate rm -f db.gen.go models.gen.go search.sql.gen.go insert.sql.gen.go get.sql.gen.go list.sql.gen.go
//go:generate go tool sqlc generate --file sqlc.yaml
//go:generate mv search.sql.go search.sql.gen.go
//go:generate mv insert.sql.go insert.sql.gen.go
//go:generate mv get.sql.go get.sql.gen.go
//go:generate mv list.sql.go list.sql.gen.go

// ErrStorageLimitExceeded is returned by InsertItemChecked when adding the new
// item would push the total stored size over the configured maximum.
var ErrStorageLimitExceeded = errors.New("storage limit exceeded")

// ErrItemAlreadyExists is returned by InsertItemChecked when an item with the
// same item_id already exists in the database.
var ErrItemAlreadyExists = errors.New("item already exists")

// ErrDuplicateChecksum is returned by InsertItemChecked when another item of
// the same type with an identical checksum already exists in the database.
var ErrDuplicateChecksum = errors.New("item with same type and checksum already exists")

// ErrDuplicateVariant is returned when the same group_key+group_value already
// exists within a group (e.g. same resolution uploaded twice).
var ErrDuplicateVariant = errors.New("variant already exists in this group")

// constraintTypeChecksum is the explicit name of the (checksum)
// unique constraint defined in the schema migration.
const constraintTypeChecksum = "asset_asset_type_checksum_key"

// constraintGroupVariant is the explicit name of the (group_id, group_key, group_value)
// unique constraint defined in the schema migration.
const constraintGroupVariant = "asset_item_group_variant_key"

// InsertItemChecked wraps the generated InsertItem and returns
// ErrStorageLimitExceeded when the storage limit would be exceeded,
// ErrDuplicateChecksum when an item with the same type+checksum exists,
// ErrDuplicateVariant when the same variant already exists in the group, or
// ErrItemAlreadyExists when the item_id is already present.
func (q *Queries) InsertItemChecked(ctx context.Context, arg InsertItemParams) error {
	rows, err := q.InsertItem(ctx, arg)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			if pgErr.ConstraintName == constraintTypeChecksum {
				return ErrDuplicateChecksum
			}
			if pgErr.ConstraintName == constraintGroupVariant {
				return ErrDuplicateVariant
			}
			return ErrItemAlreadyExists
		}
		return err
	}
	if rows == 0 {
		return ErrStorageLimitExceeded
	}
	return nil
}
