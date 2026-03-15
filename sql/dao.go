package sql

import (
	"context"
	stdsql "database/sql"
)

// DAO is the data-access interface used by the service layer.
// Every method call on a DAO returned by Tx participates in the same transaction.
type DAO interface {
	Search(ctx context.Context, arg SearchParams) ([]SearchRow, error)
	SearchByType(ctx context.Context, arg SearchByTypeParams) ([]SearchByTypeRow, error)
	ListItems(ctx context.Context, arg ListItemsParams) ([]ListItemsRow, error)
	GetItemFilePath(ctx context.Context, arg GetItemFilePathParams) (string, error)
	GetItemThumbnailPath(ctx context.Context, arg GetItemThumbnailPathParams) (stdsql.NullString, error)
	InsertItem(ctx context.Context, arg InsertItemParams) (int64, error)
	InsertItemMetadata(ctx context.Context, arg InsertItemMetadataParams) error
	InsertSearchValue(ctx context.Context, arg InsertSearchValueParams) error
	// Tx runs fn inside a single database transaction.
	// The transaction is committed when fn returns nil, rolled back otherwise.
	Tx(ctx context.Context, fn func(tx DAO) error) error
}

type dao struct {
	db *stdsql.DB
	q  *Queries
}

// NewDAO wraps a *sql.DB and its prepared *Queries into a DAO.
func NewDAO(db *stdsql.DB, q *Queries) DAO {
	return &dao{db: db, q: q}
}

func (d *dao) Search(ctx context.Context, arg SearchParams) ([]SearchRow, error) {
	return d.q.Search(ctx, arg)
}

func (d *dao) SearchByType(ctx context.Context, arg SearchByTypeParams) ([]SearchByTypeRow, error) {
	return d.q.SearchByType(ctx, arg)
}

func (d *dao) ListItems(ctx context.Context, arg ListItemsParams) ([]ListItemsRow, error) {
	return d.q.ListItems(ctx, arg)
}

func (d *dao) GetItemFilePath(ctx context.Context, arg GetItemFilePathParams) (string, error) {
	return d.q.GetItemFilePath(ctx, arg)
}

func (d *dao) GetItemThumbnailPath(ctx context.Context, arg GetItemThumbnailPathParams) (stdsql.NullString, error) {
	return d.q.GetItemThumbnailPath(ctx, arg)
}

func (d *dao) InsertItem(ctx context.Context, arg InsertItemParams) (int64, error) {
	return d.q.InsertItem(ctx, arg)
}

func (d *dao) InsertItemMetadata(ctx context.Context, arg InsertItemMetadataParams) error {
	return d.q.InsertItemMetadata(ctx, arg)
}

func (d *dao) InsertSearchValue(ctx context.Context, arg InsertSearchValueParams) error {
	return d.q.InsertSearchValue(ctx, arg)
}

func (d *dao) Tx(ctx context.Context, fn func(tx DAO) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op
	if err := fn(&dao{db: d.db, q: d.q.WithTx(tx)}); err != nil {
		return err
	}
	return tx.Commit()
}
