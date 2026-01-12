package database

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
)

type DatabaseInterface interface {
	Ping(context.Context) error
	Find(ctx context.Context, db string, collection string, filter any, opts ...any) (any, error)
	FindOne(ctx context.Context, db string, collection string, filter any, opts ...any) (any, error)
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
