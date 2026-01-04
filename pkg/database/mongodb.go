package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	moptions "go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
)

// MongoOptions holds the configuration for Mongo
type MongoOptions struct {
	Uri           string `validate:"required_without=Host"`
	Host          string `validate:"required_without=Uri"`
	AuthSource    string `validate:"required_without=Uri"`
	Username      string `validate:"required_without=Uri"`
	Password      string `validate:"required_without=Uri"`
	Timeout       int    `validate:"required,gte=0"`
	AuthMechanism string
	ReplicaSet    string
	RetryWrites   bool
}

// MongoOptionsBuilder provides a fluent interface for building Mongo options
type MongoOptionsBuilder struct {
	options *MongoOptions
}

// MongoOptions creates a new Mongo options builder
func NewMongoOptions() *MongoOptionsBuilder {
	return &MongoOptionsBuilder{
		options: &MongoOptions{},
	}
}

// SetUri set
func (b *MongoOptionsBuilder) SetUri(uri string) *MongoOptionsBuilder {
	b.options.Uri = uri
	return b
}

// SetHost sets the host
func (b *MongoOptionsBuilder) SetHost(host string) *MongoOptionsBuilder {
	b.options.Host = host
	return b
}

// SetAuthSource sets the authentication source
func (b *MongoOptionsBuilder) SetAuthSource(authSource string) *MongoOptionsBuilder {
	b.options.AuthSource = authSource
	return b
}

// SetAuthMechanism sets the authentication mechanism
func (b *MongoOptionsBuilder) SetAuthMechanism(authMechanism string) *MongoOptionsBuilder {
	b.options.AuthMechanism = authMechanism
	return b
}

// SetReplicaSet sets the replica set
func (b *MongoOptionsBuilder) SetReplicaSet(replicaSet string) *MongoOptionsBuilder {
	b.options.ReplicaSet = replicaSet
	return b
}

// SetUsername sets the username
func (b *MongoOptionsBuilder) SetUsername(username string) *MongoOptionsBuilder {
	b.options.Username = username
	return b
}

// SetPassword sets the password
func (b *MongoOptionsBuilder) SetPassword(password string) *MongoOptionsBuilder {
	b.options.Password = password
	return b
}

// SetTimeout sets the timeout
func (b *MongoOptionsBuilder) SetTimeout(timeout int) *MongoOptionsBuilder {
	b.options.Timeout = timeout
	return b
}

// SetRetryWrites sets the retry writes option
// This option was added because of DocumentDB compatibility:
// https://stackoverflow.com/questions/70260941/documentdb-mongodb-updateone-retryable-writes-are-not-supported
func (b *MongoOptionsBuilder) SetRetryWrites(retryWrites bool) *MongoOptionsBuilder {
	b.options.RetryWrites = retryWrites
	return b
}

// Build builds the Mongo options
func (b *MongoOptionsBuilder) Build() *MongoOptions {
	return b.options
}

// MongoClient wraps mongo.Client to implement DatabaseInterface
type MongoClient struct {
	Client  *mongo.Client
	Options *MongoOptions
}

// NewMongoClient creates a new MongoClient with the provided MongoDB settings
func NewMongoClient(options *MongoOptions) (DatabaseInterface, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(options.Timeout)*time.Millisecond)
	defer cancel()
	if options.Uri != "" {
		return newMongoClientFromURI(ctx, options)
	}
	return newMongoClientFromComponents(ctx, options)
}

func newMongoClientFromURI(ctx context.Context, options *MongoOptions) (DatabaseInterface, error) {
	serverAPI := moptions.ServerAPI(moptions.ServerAPIVersion1)
	opts := moptions.Client().
		ApplyURI(options.Uri).
		SetServerAPIOptions(serverAPI).
		SetRetryWrites(options.RetryWrites).
		SetMonitor(otelmongo.NewMonitor(otelmongo.WithCommandAttributeDisabled(false)))

	client, err := mongo.Connect(ctx, opts)
	return &MongoClient{
		Client:  client,
		Options: options,
	}, err
}

func newMongoClientFromComponents(ctx context.Context, options *MongoOptions) (DatabaseInterface, error) {
	// Check if host contains mongodb.net (Atlas) - use mongodb+srv://
	protocol := "mongodb://"
	if len(options.Host) > 11 && options.Host[len(options.Host)-11:] == "mongodb.net" {
		protocol = "mongodb+srv://"
	}

	uri := fmt.Sprintf("%s%s:%s@%s", protocol, options.Username, options.Password, options.Host)
	// Specify the ReplicaSet if provided (not needed for SRV)
	if options.ReplicaSet != "" {
		uri = fmt.Sprintf("%s/?replicaSet=%s", uri, options.ReplicaSet)
	}

	// Default to SCRAM-SHA-256 if no AuthMechanism is provided
	if options.AuthMechanism == "" {
		options.AuthMechanism = "SCRAM-SHA-256"
	}

	clientOpts := moptions.Client().
		ApplyURI(uri).
		SetRetryWrites(options.RetryWrites).
		SetAuth(moptions.Credential{
			AuthMechanism: options.AuthMechanism,
			AuthSource:    options.AuthSource,
			Username:      options.Username,
			Password:      options.Password,
		})

	// Add ServerAPI for Atlas connections
	if protocol == "mongodb+srv://" {
		serverAPI := moptions.ServerAPI(moptions.ServerAPIVersion1)
		clientOpts.SetServerAPIOptions(serverAPI)
	}

	client, err := mongo.Connect(ctx, clientOpts)
	return &MongoClient{
		Client:  client,
		Options: options,
	}, err
}

func (m *MongoClient) Ping(ctx context.Context) error {
	err := m.Client.Ping(ctx, nil)
	return err
}
