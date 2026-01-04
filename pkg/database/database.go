package database

import (
	"context"

	"github.com/go-playground/validator/v10"
)

type DatabaseInterface interface {
	Ping(context.Context) error
}

// Database represents a database client instance
type Database struct {
	Options *MongoOptions
	Client  DatabaseInterface
}

func New(opts *MongoOptions, client ...DatabaseInterface) (*Database, error) {
	// Validate Database configuration
	validate := validator.New()
	err := validate.Struct(opts)
	if err != nil {
		return nil, err
	}

	// If no client provided, create default production client
	var m DatabaseInterface
	if len(client) == 0 {
		m, err = NewMongoClient(opts)
	} else {
		m, err = client[0], nil
	}

	return &Database{
		Options: opts,
		Client:  m,
	}, err
}
