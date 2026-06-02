package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"go.mongodb.org/mongo-driver/bson"
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

	// TLS enables a TLS/SSL connection to the server. This is required for
	// MongoDB-compatible services that enforce in-transit encryption, such as
	// AWS DocumentDB with TLS turned on.
	TLS bool
	// TLSCAFile is the path to a PEM file containing one or more certificate
	// authorities to trust when making a TLS connection. For AWS DocumentDB this
	// is the global RDS CA bundle (e.g. global-bundle.pem).
	TLSCAFile string
	// TLSInsecureSkipVerify disables certificate and hostname verification.
	// This is insecure and should only be used for local testing.
	TLSInsecureSkipVerify bool
}

// Validate validates the MongoOptions configuration
func (m *MongoOptions) Validate() error {
	validate := validator.New()
	return validate.Struct(m)
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

// SetTLS enables or disables a TLS/SSL connection to the server.
// Enable this for MongoDB-compatible services that enforce in-transit
// encryption, such as AWS DocumentDB with TLS turned on.
func (b *MongoOptionsBuilder) SetTLS(tls bool) *MongoOptionsBuilder {
	b.options.TLS = tls
	return b
}

// SetTLSCAFile sets the path to a PEM file containing one or more certificate
// authorities to trust when establishing a TLS connection. Setting this also
// implicitly enables TLS. For AWS DocumentDB use the global RDS CA bundle.
func (b *MongoOptionsBuilder) SetTLSCAFile(caFile string) *MongoOptionsBuilder {
	b.options.TLSCAFile = caFile
	if caFile != "" {
		b.options.TLS = true
	}
	return b
}

// SetTLSInsecureSkipVerify disables certificate and hostname verification.
// This is insecure and should only be used for local testing.
func (b *MongoOptionsBuilder) SetTLSInsecureSkipVerify(skip bool) *MongoOptionsBuilder {
	b.options.TLSInsecureSkipVerify = skip
	return b
}

// Build builds the Mongo options
func (b *MongoOptionsBuilder) Build() *MongoOptions {
	return b.options
}

// Projection provides a fluent API for building MongoDB projections without exposing bson
type Projection struct {
	fields bson.M
}

// NewProjection creates a new Projection builder
func NewProjection() *Projection {
	return &Projection{
		fields: bson.M{},
	}
}

// Include adds fields to include in the result (value: 1)
func (p *Projection) Include(fields ...string) *Projection {
	for _, field := range fields {
		p.fields[field] = 1
	}
	return p
}

// Exclude adds fields to exclude from the result (value: 0)
func (p *Projection) Exclude(fields ...string) *Projection {
	for _, field := range fields {
		p.fields[field] = 0
	}
	return p
}

// toBSON converts the projection to bson.M for internal use
func (p *Projection) toBSON() bson.M {
	return p.fields
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

	if err := applyTLSConfig(opts, options); err != nil {
		return nil, err
	}

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

	if err := applyTLSConfig(clientOpts, options); err != nil {
		return nil, err
	}

	client, err := mongo.Connect(ctx, clientOpts)
	return &MongoClient{
		Client:  client,
		Options: options,
	}, err
}

// applyTLSConfig configures TLS on the provided client options when TLS is
// enabled. It optionally loads a custom CA bundle (required by managed services
// such as AWS DocumentDB) and supports skipping verification for local testing.
func applyTLSConfig(clientOpts *moptions.ClientOptions, options *MongoOptions) error {
	if !options.TLS && options.TLSCAFile == "" {
		return nil
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: options.TLSInsecureSkipVerify,
	}

	if options.TLSCAFile != "" {
		caBytes, err := os.ReadFile(options.TLSCAFile)
		if err != nil {
			return fmt.Errorf("failed to read TLS CA file %q: %w", options.TLSCAFile, err)
		}

		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caBytes) {
			return fmt.Errorf("failed to parse any certificates from TLS CA file %q", options.TLSCAFile)
		}
		tlsConfig.RootCAs = caPool
	}

	clientOpts.SetTLSConfig(tlsConfig)
	return nil
}

// Ping pings the MongoDB server to check connectivity
func (m *MongoClient) Ping(ctx context.Context) error {
	err := m.Client.Ping(ctx, nil)
	return err
}

// Disconnect disconnects the MongoDB client
func (m *MongoClient) Disconnect(ctx context.Context) error {
	return m.Client.Disconnect(ctx)
}

// GetTimeout returns the timeout duration for the MongoDB client
func (m *MongoClient) GetTimeout() time.Duration {
	return time.Duration(m.Options.Timeout) * time.Millisecond
}

// FindResult wraps a MongoDB cursor for fluent API usage
type FindResult struct {
	cursor *mongo.Cursor
	ctx    context.Context
	err    error
}

// All decodes all results into the provided destination slice.
// The dest parameter must be a pointer to a slice.
func (fr *FindResult) All(dest any) error {
	if fr.err != nil {
		return fr.err
	}
	if fr.cursor == nil {
		return fr.err
	}
	defer fr.cursor.Close(fr.ctx)
	return fr.cursor.All(fr.ctx, dest)
}

// Err returns any error that occurred during the query.
func (fr *FindResult) Err() error {
	return fr.err
}

// Find executes a find query on the specified database and collection.
// Returns a FindResult that can be used with .All() for fluent decoding.
// Supports *moptions.FindOptions and *Projection in opts.
func (m *MongoClient) Find(ctx context.Context, db string, collection string, filter any, opts ...any) FindResultInterface {
	coll := m.Client.Database(db).Collection(collection)

	// Convert opts to mongo.FindOptions if provided
	var findOpts []*moptions.FindOptions
	for _, opt := range opts {
		if fo, ok := opt.(*moptions.FindOptions); ok {
			findOpts = append(findOpts, fo)
		} else if proj, ok := opt.(*Projection); ok {
			// Convert Projection to FindOptions with SetProjection
			findOpts = append(findOpts, moptions.Find().SetProjection(proj.toBSON()))
		}
	}

	cursor, err := coll.Find(ctx, filter, findOpts...)
	return &FindResult{cursor: cursor, ctx: ctx, err: err}
}

// FindOne executes a findOne query on the specified database and collection.
// Returns a SingleResult that can be used with .Into() or .Raw() for fluent decoding.
// Supports *moptions.FindOneOptions and *Projection in opts.
func (m *MongoClient) FindOne(ctx context.Context, db string, collection string, filter any, opts ...any) SingleResultInterface {
	coll := m.Client.Database(db).Collection(collection)

	// Convert opts to mongo.FindOneOptions if provided
	var findOneOpts []*moptions.FindOneOptions
	for _, opt := range opts {
		if fo, ok := opt.(*moptions.FindOneOptions); ok {
			findOneOpts = append(findOneOpts, fo)
		} else if proj, ok := opt.(*Projection); ok {
			// Convert Projection to FindOneOptions with SetProjection
			findOneOpts = append(findOneOpts, moptions.FindOne().SetProjection(proj.toBSON()))
		}
	}

	return &SingleResult{result: coll.FindOne(ctx, filter, findOneOpts...)}
}

// InsertOne inserts a single document into the specified database and collection
func (m *MongoClient) InsertOne(ctx context.Context, db string, collection string, document any, opts ...any) (any, error) {
	coll := m.Client.Database(db).Collection(collection)

	result, err := coll.InsertOne(ctx, document)
	if err != nil {
		return nil, err
	}

	return result.InsertedID, nil
}

// InsertMany inserts multiple documents into the specified database and collection
func (m *MongoClient) InsertMany(ctx context.Context, db string, collection string, documents []any, opts ...any) (any, error) {
	coll := m.Client.Database(db).Collection(collection)

	result, err := coll.InsertMany(ctx, documents)
	if err != nil {
		return nil, err
	}

	return result.InsertedIDs, nil
}

// SingleResult wraps a MongoDB single result for fluent API usage
type SingleResult struct {
	result *mongo.SingleResult
}

// Into decodes the result directly into the provided destination.
// The dest parameter must be a pointer to the struct you want to decode into.
func (sr *SingleResult) Into(dest any) error {
	return sr.result.Decode(dest)
}

// Raw returns the result as a raw any type (primitive.D).
// This is useful when you need the raw BSON document.
func (sr *SingleResult) Raw() (any, error) {
	var result any
	err := sr.result.Decode(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Err returns any error that occurred during the query.
// Returns mongo.ErrNoDocuments if no document was found.
func (sr *SingleResult) Err() error {
	return sr.result.Err()
}

// UpdateResult wraps a MongoDB update result for fluent API usage
type UpdateResult struct {
	result *mongo.UpdateResult
}

// DeleteResult wraps a MongoDB delete result for fluent API usage
type DeleteResult struct {
	result *mongo.DeleteResult
}

// MatchedCount returns the number of documents matched by the filter
func (ur *UpdateResult) MatchedCount() int64 {
	return ur.result.MatchedCount
}

// ModifiedCount returns the number of documents modified by the operation
func (ur *UpdateResult) ModifiedCount() int64 {
	return ur.result.ModifiedCount
}

// UpsertedCount returns the number of documents upserted by the operation
func (ur *UpdateResult) UpsertedCount() int64 {
	return ur.result.UpsertedCount
}

// UpsertedID returns the _id of the upserted document, or nil if no upsert occurred
func (ur *UpdateResult) UpsertedID() any {
	return ur.result.UpsertedID
}

// DeletedCount returns the number of documents deleted by the operation
func (dr *DeleteResult) DeletedCount() int64 {
	return dr.result.DeletedCount
}

// Count returns the number of documents in the specified database and collection matching the filter.
// Supports *moptions.CountOptions in opts.
func (m *MongoClient) Count(ctx context.Context, db string, collection string, filter any, opts ...any) (int64, error) {
	coll := m.Client.Database(db).Collection(collection)

	var countOpts []*moptions.CountOptions
	for _, opt := range opts {
		if co, ok := opt.(*moptions.CountOptions); ok {
			countOpts = append(countOpts, co)
		}
	}

	return coll.CountDocuments(ctx, filter, countOpts...)
}

// UpdateOne executes an update query on a single document in the specified database and collection.
// Returns an UpdateResult that provides access to matched, modified, and upserted counts.
// Supports *moptions.UpdateOptions in opts.
func (m *MongoClient) UpdateOne(ctx context.Context, db string, collection string, filter any, update any, opts ...any) (UpdateResultInterface, error) {
	coll := m.Client.Database(db).Collection(collection)

	// Convert opts to mongo.UpdateOptions if provided
	var updateOpts []*moptions.UpdateOptions
	for _, opt := range opts {
		if uo, ok := opt.(*moptions.UpdateOptions); ok {
			updateOpts = append(updateOpts, uo)
		}
	}

	result, err := coll.UpdateOne(ctx, filter, update, updateOpts...)
	if err != nil {
		return nil, err
	}

	return &UpdateResult{result: result}, nil
}

// DeleteOne executes a delete query on a single document in the specified database and collection.
// Returns a DeleteResult that provides access to the number of deleted documents.
// Supports *moptions.DeleteOptions in opts.
func (m *MongoClient) DeleteOne(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error) {
	coll := m.Client.Database(db).Collection(collection)

	var deleteOpts []*moptions.DeleteOptions
	for _, opt := range opts {
		if do, ok := opt.(*moptions.DeleteOptions); ok {
			deleteOpts = append(deleteOpts, do)
		}
	}

	result, err := coll.DeleteOne(ctx, filter, deleteOpts...)
	if err != nil {
		return nil, err
	}

	return &DeleteResult{result: result}, nil
}

// DeleteMany executes a delete query on multiple documents in the specified database and collection.
// Returns a DeleteResult that provides access to the number of deleted documents.
// Supports *moptions.DeleteOptions in opts.
func (m *MongoClient) DeleteMany(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error) {
	coll := m.Client.Database(db).Collection(collection)

	var deleteOpts []*moptions.DeleteOptions
	for _, opt := range opts {
		if do, ok := opt.(*moptions.DeleteOptions); ok {
			deleteOpts = append(deleteOpts, do)
		}
	}

	result, err := coll.DeleteMany(ctx, filter, deleteOpts...)
	if err != nil {
		return nil, err
	}

	return &DeleteResult{result: result}, nil
}
