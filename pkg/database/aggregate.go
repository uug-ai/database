package database

import "context"

// Aggregator is the optional database capability for executing MongoDB
// aggregation pipelines. Pipeline construction and result decoding remain
// consumer concerns.
type Aggregator interface {
	Aggregate(ctx context.Context, db string, collection string, pipeline any, opts ...any) FindResultInterface
}
