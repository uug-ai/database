package database

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
)

// IndexManager is the optional database capability for creating MongoDB
// collection indexes. Index definitions remain owned by consumers so this
// package does not need domain-specific index registries.
type IndexManager interface {
	CreateIndexes(ctx context.Context, db string, collection string, indexes []mongo.IndexModel, opts ...any) ([]string, error)
}
