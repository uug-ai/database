package database

import "github.com/go-playground/validator/v10"

type DatabaseInterface interface {
	Ping() error
}

// SMTP represents an SMTP client instance
type Database struct {
	options *MongoOptions
	client  DatabaseInterface
}

func New(opts *MongoOptions, client ...DatabaseInterface) (*Database, error) {
	// Validate SMTP configuration
	validate := validator.New()
	err := validate.Struct(opts)
	if err != nil {
		return nil, err
	}

	// If no client provided, create default production client
	var m DatabaseInterface
	if len(client) == 0 {
		m = NewMongoClient(opts)
	} else {
		m = client[0]
	}

	return &Database{
		options: opts,
		client:  m,
	}, nil
}
