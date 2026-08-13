package database

import "context"

// MultiUpdater is the optional database capability for updating multiple
// documents. Filters and update documents remain owned by consumers.
type MultiUpdater interface {
	UpdateMany(ctx context.Context, db string, collection string, filter any, update any, opts ...any) (UpdateResultInterface, error)
}
