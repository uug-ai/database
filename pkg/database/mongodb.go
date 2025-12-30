package database

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	moptions "go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
)

// MongoOptions holds the configuration for Mongo
type MongoOptions struct {
	Uri           string `validate:"required"`
	Host          string `validate:"required"`
	AuthSource    string `validate:"required"`
	AuthMechanism string `validate:"required"`
	ReplicaSet    string `validate:"required"`
	Username      string `validate:"required"`
	Password      string `validate:"required"`
	Timeout       int    `validate:"required,gte=0"`
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
	options *MongoOptions
}

// NewMongoClient creates a new MongoClient with the provided MongoDB settings
func NewMongoClient(options *MongoOptions) DatabaseInterface {

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(options.Timeout)*time.Millisecond)
	defer cancel()

	// We can also apply the complete URI
	// e.g. "mongodb+srv://<username>:<password>@kerberos-hub.shhng.mongodb.net/?retryWrites=true&w=majority&appName=kerberos-hub"
	if options.Uri != "" {
		serverAPI := moptions.ServerAPI(moptions.ServerAPIVersion1)
		opts := moptions.Client().
			ApplyURI(options.Uri).
			SetServerAPIOptions(serverAPI).
			SetRetryWrites(options.RetryWrites).
			SetMonitor(otelmongo.NewMonitor(otelmongo.WithCommandAttributeDisabled(false)))

		// Create a new client and connect to the server
		client, err := mongo.Connect(ctx, opts)
		if err != nil {
			fmt.Printf("Error setting up mongodb connection: %+v\n", err)
			os.Exit(1)
		}

		return &MongoClient{
			Client: client,
		}

	}

	// If we don't have a full URI, build it from components such as host, username, password, etc.
	// This will give less flexibility than a full URI, but is provided for convenience.
	uri := fmt.Sprintf("mongodb://%s:%s@%s", options.Username, options.Password, options.Host)
	if options.ReplicaSet != "" {
		uri = fmt.Sprintf("%s/?replicaSet=%s", uri, options.ReplicaSet)
	}
	if options.AuthMechanism == "" {
		options.AuthMechanism = "SCRAM-SHA-256"
	}
	client, err := mongo.Connect(ctx, moptions.Client().
		ApplyURI(uri).
		SetRetryWrites(options.RetryWrites).
		SetAuth(moptions.Credential{
			AuthMechanism: options.AuthMechanism,
			AuthSource:    options.AuthSource,
			Username:      options.Username,
			Password:      options.Password,
		}))

	if err != nil {
		fmt.Printf("Error setting up mongodb connection: %+v\n", err)
		os.Exit(1)
	}

	return &MongoClient{
		Client: client,
	}
}

func (m *MongoClient) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.options.Timeout)*time.Millisecond)
	defer cancel()
	err := m.Client.Ping(ctx, nil)
	return err
}
