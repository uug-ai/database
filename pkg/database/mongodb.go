package database

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

// MongoClient wraps mongo.Client to implement DatabaseInterface
type MongoClient struct {
	Client *mongo.Client
}

var (
	TIMEOUT = 10 * time.Second
)

// NewMongoClient creates a new MongoClient with the provided MongoDB settings
func NewMongoClient(uri string,
	host string,
	port int,
	username string,
	password string) DatabaseInterface {

	// We can also apply the complete URI
	// e.g. "mongodb+srv://<username>:<password>@kerberos-hub.shhng.mongodb.net/?retryWrites=true&w=majority&appName=kerberos-hub"
	if uri != "" {
		serverAPI := options.ServerAPI(options.ServerAPIVersion1)
		opts := options.Client().
			ApplyURI(uri).
			SetServerAPIOptions(serverAPI).
			SetRetryWrites(retryWrites).
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

	} else {

		// New MongoDB driver
		uri := fmt.Sprintf("mongodb://%s:%s@%s", username, password, host)
		if replicaset != "" {
			uri = fmt.Sprintf("%s/?replicaSet=%s", uri, replicaset)
		}
		if authenticationMechanism == "" {
			authenticationMechanism = "SCRAM-SHA-256"
		}
		client, err := mongo.Connect(ctx, options.Client().
			ApplyURI(uri).
			SetRetryWrites(retryWrites).
			SetAuth(options.Credential{
				AuthMechanism: authenticationMechanism,
				AuthSource:    databaseCredentials,
				Username:      username,
				Password:      password,
			}))

		if err != nil {
			fmt.Printf("Error setting up mongodb connection: %+v\n", err)
			os.Exit(1)
		}

		return &MongoClient{
			Client: client,
		}
	}
}

func (m *MongoClient) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	err := m.Client.Ping(ctx, nil)
	return err
}

// MongoOptions holds the configuration for Mongo
type MongoOptions struct {
	Uri                 string `validate:"required"`
	Host                int    `validate:"required,gt=0"`
	DatabaseCredentials string `validate:"required"`
	ReplicaSet          string `validate:"required"`
	Username            string `validate:"required"`
	Password            string `validate:"required"`
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
func (b *MongoOptionsBuilder) SetServer(server string) *MongoOptionsBuilder {
	b.options.Server = server
	return b
}

// SetPort sets the SMTP server port
func (b *MongoOptionsBuilder) SetPort(port int) *MongoOptionsBuilder {
	b.options.Port = port
	return b
}

// SMTP represents an SMTP client instance
type SMTP struct {
	options *MongoOptions
	client  DatabaseInterface
}

func NewMongo(opts *MongoOptions, client ...DatabaseInterface) (*SMTP, error) {
	// Validate SMTP configuration
	validate := validator.New()
	err := validate.Struct(opts)
	if err != nil {
		return nil, err
	}

	// If no client provided, create default production client
	var c MailClient
	if len(client) == 0 {
		c = NewGomailClient(opts.Server, opts.Port, opts.Username, opts.Password)
	} else {
		c = client[0]
	}

	return &SMTP{
		options: opts,
		client:  c,
	}, nil
}

/*type DB struct {
	Client *mongo.Client
}

var TIMEOUT = 10 * time.Second
var _init_ctx sync.Once
var _instance *DB

var DatabaseName = os.Getenv("MONGODB_DATABASE_CLOUD")

func New() *DB {

	mongodbURI := os.Getenv("MONGODB_URI")
	host := os.Getenv("MONGODB_HOST")
	databaseCredentials := os.Getenv("MONGODB_DATABASE_CREDENTIALS")
	replicaset := os.Getenv("MONGODB_REPLICASET")
	username := os.Getenv("MONGODB_USERNAME")
	password := os.Getenv("MONGODB_PASSWORD")
	authenticationMechanism := os.Getenv("MONGODB_AUTHENTICATION_MECHANISM")
	retryWrites := os.Getenv("MONGODB_RETRY_WRITES") != "false" // Default to true unless explicitly set to "false"

	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	_init_ctx.Do(func() {
		_instance = new(DB)

		// We can also apply the complete URI
		// e.g. "mongodb+srv://<username>:<password>@kerberos-hub.shhng.mongodb.net/?retryWrites=true&w=majority&appName=kerberos-hub"
		if mongodbURI != "" {
			serverAPI := options.ServerAPI(options.ServerAPIVersion1)
			opts := options.Client().
				ApplyURI(mongodbURI).
				SetServerAPIOptions(serverAPI).
				SetRetryWrites(retryWrites).
				SetMonitor(otelmongo.NewMonitor(otelmongo.WithCommandAttributeDisabled(false)))

			// Create a new client and connect to the server
			client, err := mongo.Connect(ctx, opts)
			if err != nil {
				fmt.Printf("Error setting up mongodb connection: %+v\n", err)
				os.Exit(1)
			}
			_instance.Client = client

		} else {

			// New MongoDB driver
			mongodbURI := fmt.Sprintf("mongodb://%s:%s@%s", username, password, host)
			if replicaset != "" {
				mongodbURI = fmt.Sprintf("%s/?replicaSet=%s", mongodbURI, replicaset)
			}
			if authenticationMechanism == "" {
				authenticationMechanism = "SCRAM-SHA-256"
			}
			client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongodbURI).SetRetryWrites(retryWrites).SetAuth(options.Credential{
				AuthMechanism: authenticationMechanism,
				AuthSource:    databaseCredentials,
				Username:      username,
				Password:      password,
			}))
			if err != nil {
				fmt.Printf("Error setting up mongodb connection: %+v\n", err)
				os.Exit(1)
			}
			_instance.Client = client
		}
	})

	return _instance
}

*/
