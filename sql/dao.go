package sql

import (
	"context"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DAO is the data-access interface used by the service layer.
// Every method call on a DAO returned by Tx participates in the same transaction.
type DAO interface {
	Search(ctx context.Context, arg SearchParams) ([]SearchRow, error)
	SearchByType(ctx context.Context, arg SearchByTypeParams) ([]SearchByTypeRow, error)
	ListItems(ctx context.Context, arg ListItemsParams) ([]ListItemsRow, error)
	GetItemFilePath(ctx context.Context, arg GetItemFilePathParams) (GetItemFilePathRow, error)
	GetGroupFilePath(ctx context.Context, arg GetGroupFilePathParams) (GetGroupFilePathRow, error)
	GetItemThumbnailPath(ctx context.Context, arg GetItemThumbnailPathParams) (GetItemThumbnailPathRow, error)
	GetGroupThumbnailPath(ctx context.Context, groupID pgtype.UUID) (GetGroupThumbnailPathRow, error)
	GetItemByChecksum(ctx context.Context, checksum string) (GetItemByChecksumRow, error)
	GetGroupFiles(ctx context.Context, arg GetGroupFilesParams) ([]GetGroupFilesRow, error)
	GetMultiGroupFiles(ctx context.Context, groupIDs []pgtype.UUID) ([]GetMultiGroupFilesRow, error)
	UpsertGroup(ctx context.Context, arg UpsertGroupParams) error
	GetGroupID(ctx context.Context, arg GetGroupIDParams) (pgtype.UUID, error)
	InsertItem(ctx context.Context, arg InsertItemParams) (int64, error)
	InsertItemChecked(ctx context.Context, arg InsertItemParams) error
	InsertItemMetadata(ctx context.Context, arg InsertItemMetadataParams) error
	InsertSearchValue(ctx context.Context, arg InsertSearchValueParams) error

	// Admin operations
	DeleteGroup(ctx context.Context, arg DeleteGroupParams) error
	DeleteItem(ctx context.Context, arg DeleteItemParams) error
	UpdateGroupName(ctx context.Context, arg UpdateGroupNameParams) error
	DeleteSearchValues(ctx context.Context, arg DeleteSearchValuesParams) error
	GetGroupInfo(ctx context.Context, arg GetGroupInfoParams) (AssetGroup, error)
	GetItemInfo(ctx context.Context, arg GetItemInfoParams) (GetItemInfoRow, error)
	GetGroupItemPaths(ctx context.Context, groupID pgtype.UUID) ([]GetGroupItemPathsRow, error)
	GetGroupItems(ctx context.Context, groupID pgtype.UUID) ([]GetGroupItemsRow, error)
	GetGroupItemsWithMetadata(ctx context.Context, groupID pgtype.UUID) ([]GetGroupItemsWithMetadataRow, error)
	UpdateItem(ctx context.Context, arg UpdateItemParams) error
	CountGroupItems(ctx context.Context, groupID pgtype.UUID) (int64, error)
	CountGroupsCreatedByIP(ctx context.Context, addr netip.Addr, since time.Time) (int64, error)

	// Tx runs fn inside a single database transaction.
	// The transaction is committed when fn returns nil, rolled back otherwise.
	Tx(ctx context.Context, fn func(tx DAO) error) error
}

type dao struct {
	pool *pgxpool.Pool
	q    *Queries
}

// NewDAO wraps a *pgxpool.Pool and its *Queries into a DAO.
func NewDAO(pool *pgxpool.Pool) DAO {
	return &dao{pool: pool, q: New(pool)}
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

func (d *dao) GetItemFilePath(ctx context.Context, arg GetItemFilePathParams) (GetItemFilePathRow, error) {
	return d.q.GetItemFilePath(ctx, arg)
}

func (d *dao) GetGroupFilePath(ctx context.Context, arg GetGroupFilePathParams) (GetGroupFilePathRow, error) {
	return d.q.GetGroupFilePath(ctx, arg)
}

func (d *dao) GetItemThumbnailPath(ctx context.Context, arg GetItemThumbnailPathParams) (GetItemThumbnailPathRow, error) {
	return d.q.GetItemThumbnailPath(ctx, arg)
}

func (d *dao) GetGroupThumbnailPath(ctx context.Context, groupID pgtype.UUID) (GetGroupThumbnailPathRow, error) {
	return d.q.GetGroupThumbnailPath(ctx, groupID)
}

func (d *dao) GetItemByChecksum(ctx context.Context, checksum string) (GetItemByChecksumRow, error) {
	return d.q.GetItemByChecksum(ctx, checksum)
}

func (d *dao) GetGroupFiles(ctx context.Context, arg GetGroupFilesParams) ([]GetGroupFilesRow, error) {
	return d.q.GetGroupFiles(ctx, arg)
}

func (d *dao) GetMultiGroupFiles(ctx context.Context, groupIDs []pgtype.UUID) ([]GetMultiGroupFilesRow, error) {
	return d.q.GetMultiGroupFiles(ctx, groupIDs)
}

func (d *dao) UpsertGroup(ctx context.Context, arg UpsertGroupParams) error {
	return d.q.UpsertGroup(ctx, arg)
}

func (d *dao) GetGroupID(ctx context.Context, arg GetGroupIDParams) (pgtype.UUID, error) {
	return d.q.GetGroupID(ctx, arg)
}

func (d *dao) InsertItem(ctx context.Context, arg InsertItemParams) (int64, error) {
	return d.q.InsertItem(ctx, arg)
}

func (d *dao) InsertItemChecked(ctx context.Context, arg InsertItemParams) error {
	return d.q.InsertItemChecked(ctx, arg)
}

func (d *dao) InsertItemMetadata(ctx context.Context, arg InsertItemMetadataParams) error {
	return d.q.InsertItemMetadata(ctx, arg)
}

func (d *dao) InsertSearchValue(ctx context.Context, arg InsertSearchValueParams) error {
	return d.q.InsertSearchValue(ctx, arg)
}

func (d *dao) DeleteGroup(ctx context.Context, arg DeleteGroupParams) error {
	return d.q.DeleteGroup(ctx, arg)
}

func (d *dao) DeleteItem(ctx context.Context, arg DeleteItemParams) error {
	return d.q.DeleteItem(ctx, arg)
}

func (d *dao) UpdateGroupName(ctx context.Context, arg UpdateGroupNameParams) error {
	return d.q.UpdateGroupName(ctx, arg)
}

func (d *dao) DeleteSearchValues(ctx context.Context, arg DeleteSearchValuesParams) error {
	return d.q.DeleteSearchValues(ctx, arg)
}

func (d *dao) GetGroupInfo(ctx context.Context, arg GetGroupInfoParams) (AssetGroup, error) {
	return d.q.GetGroupInfo(ctx, arg)
}

func (d *dao) GetItemInfo(ctx context.Context, arg GetItemInfoParams) (GetItemInfoRow, error) {
	return d.q.GetItemInfo(ctx, arg)
}

func (d *dao) GetGroupItemPaths(ctx context.Context, groupID pgtype.UUID) ([]GetGroupItemPathsRow, error) {
	return d.q.GetGroupItemPaths(ctx, groupID)
}

func (d *dao) GetGroupItems(ctx context.Context, groupID pgtype.UUID) ([]GetGroupItemsRow, error) {
	return d.q.GetGroupItems(ctx, groupID)
}

func (d *dao) GetGroupItemsWithMetadata(ctx context.Context, groupID pgtype.UUID) ([]GetGroupItemsWithMetadataRow, error) {
	return d.q.GetGroupItemsWithMetadata(ctx, groupID)
}

func (d *dao) UpdateItem(ctx context.Context, arg UpdateItemParams) error {
	return d.q.UpdateItem(ctx, arg)
}

func (d *dao) CountGroupItems(ctx context.Context, groupID pgtype.UUID) (int64, error) {
	return d.q.CountGroupItems(ctx, groupID)
}

func (d *dao) CountGroupsCreatedByIP(ctx context.Context, addr netip.Addr, since time.Time) (int64, error) {
	if !addr.IsValid() {
		return 0, nil
	}
	return d.q.CountGroupsCreatedByIP(ctx, CountGroupsCreatedByIPParams{
		CreatorIp: addr,
		Since:     pgtype.Timestamptz{Time: since, Valid: true},
	})
}

func (d *dao) Tx(ctx context.Context, fn func(tx DAO) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op
	if err := fn(&dao{pool: d.pool, q: d.q.WithTx(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
