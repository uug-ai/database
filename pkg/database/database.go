package database

import (
	"context"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
)

// SingleResultInterface defines the interface for single query results
type SingleResultInterface interface {
	Into(dest any) error
	Raw() (any, error)
	Err() error
}

// FindResultInterface defines the interface for find query results
type FindResultInterface interface {
	All(dest any) error
	Err() error
}

// CursorResultInterface is the optional per-document iteration capability for
// find results. Consumers use it when individual decode failures must be
// handled without aborting the complete result set.
type CursorResultInterface interface {
	FindResultInterface
	Next(ctx context.Context) bool
	Decode(dest any) error
	Close(ctx context.Context) error
}

// UpdateResultInterface defines the interface for update operation results
type UpdateResultInterface interface {
	MatchedCount() int64
	ModifiedCount() int64
	UpsertedCount() int64
	UpsertedID() any
}

// DeleteResultInterface defines the interface for delete operation results
type DeleteResultInterface interface {
	DeletedCount() int64
}

type DatabaseInterface interface {
	GetTimeout() time.Duration
	Ping(context.Context) error
	Find(ctx context.Context, db string, collection string, filter any, opts ...any) FindResultInterface
	FindOne(ctx context.Context, db string, collection string, filter any, opts ...any) SingleResultInterface
	FindOneAndUpdate(ctx context.Context, db string, collection string, filter any, update any, opts ...any) SingleResultInterface
	UpdateOne(ctx context.Context, db string, collection string, filter any, update any, opts ...any) (UpdateResultInterface, error)
	DeleteOne(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error)
	DeleteMany(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error)
	Count(ctx context.Context, db string, collection string, filter any, opts ...any) (int64, error)
	Disconnect(ctx context.Context) error
	InsertOne(ctx context.Context, db string, collection string, document any, opts ...any) (any, error)
	InsertMany(ctx context.Context, db string, collection string, documents []any, opts ...any) (any, error)
}

type DatabaseOptions interface {
	Validate() error
}

// Database represents a database client instance
type Database struct {
	Options DatabaseOptions
	Client  DatabaseInterface
}

func New(opts DatabaseOptions, client ...DatabaseInterface) (*Database, error) {
	// Validate Database configuration
	validate := validator.New()
	err := validate.Struct(opts)
	if err != nil {
		return nil, err
	}

	// If no client provided, create default production client
	var m DatabaseInterface
	if len(client) == 0 {
		// Type assert to RabbitOptions for creating RabbitMQ client
		if mongoOpts, ok := opts.(*MongoOptions); ok {
			m, err = NewMongoClient(mongoOpts)
		} else {
			return nil, fmt.Errorf("unsupported queue options type")
		}
	} else {
		m, err = client[0], nil
	}

	return &Database{
		Options: opts,
		Client:  m,
	}, err
}
