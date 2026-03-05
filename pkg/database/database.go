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

// UpdateResultInterface defines the interface for update operation results
type UpdateResultInterface interface {
	MatchedCount() int64
	ModifiedCount() int64
	UpsertedCount() int64
	UpsertedID() any
}

type DatabaseInterface interface {
	GetTimeout() time.Duration
	Ping(context.Context) error
	Find(ctx context.Context, db string, collection string, filter any, opts ...any) FindResultInterface
	FindOne(ctx context.Context, db string, collection string, filter any, opts ...any) SingleResultInterface
	UpdateOne(ctx context.Context, db string, collection string, filter any, update any, opts ...any) (UpdateResultInterface, error)
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
