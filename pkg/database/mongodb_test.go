package database

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uug-ai/models/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	moptions "go.mongodb.org/mongo-driver/mongo/options"
)

func TestInsertOptions(t *testing.T) {
	t.Run("InsertOneRetainsSupportedOptions", func(t *testing.T) {
		first := moptions.InsertOne().SetBypassDocumentValidation(true)
		second := moptions.InsertOne().SetComment("insert-one")

		got := insertOneOptions(first, moptions.Find(), second)
		if len(got) != 2 || got[0] != first || got[1] != second {
			t.Fatalf("insert one options = %#v", got)
		}
	})

	t.Run("InsertManyRetainsSupportedOptions", func(t *testing.T) {
		first := moptions.InsertMany().SetOrdered(false)
		second := moptions.InsertMany().SetBypassDocumentValidation(true)

		got := insertManyOptions(first, moptions.FindOne(), second)
		if len(got) != 2 || got[0] != first || got[1] != second {
			t.Fatalf("insert many options = %#v", got)
		}
	})
}

func TestAggregateOptions(t *testing.T) {
	first := moptions.Aggregate().SetAllowDiskUse(true)
	second := moptions.Aggregate().SetBatchSize(100)

	got := aggregateOptions(first, moptions.Find(), second)
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("aggregate options = %#v", got)
	}
}

func envVarsSet(keys ...string) bool {
	for _, key := range keys {
		if os.Getenv(key) == "" {
			return false
		}
	}
	return true
}

// TestMongoOptionsValidation tests the validation of MongoDB options
func TestMongoOptionsValidation(t *testing.T) {
	tests := []struct {
		name        string
		buildOpts   func() *MongoOptions
		expectError bool
	}{
		{
			name: "ValidOptionsWithURI",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetUri("mongodb://user:pass@localhost:27017").
					SetTimeout(5000).
					Build()
			},
			expectError: false,
		},
		{
			name: "ValidOptionsWithComponents",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetHost("localhost").
					SetAuthSource("admin").
					SetAuthMechanism("SCRAM-SHA-256").
					SetReplicaSet("rs0").
					SetUsername("user").
					SetPassword("pass").
					SetTimeout(5000).
					Build()
			},
			expectError: false,
		},
		{
			name: "MissingUriAndHost",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetAuthSource("admin").
					SetAuthMechanism("SCRAM-SHA-256").
					SetReplicaSet("rs0").
					SetUsername("user").
					SetPassword("pass").
					SetTimeout(5000).
					Build()
			},
			expectError: true,
		},
		{
			name: "MissingHostWhenNoUri",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetAuthSource("admin").
					SetAuthMechanism("SCRAM-SHA-256").
					SetReplicaSet("rs0").
					SetUsername("user").
					SetPassword("pass").
					SetTimeout(5000).
					Build()
			},
			expectError: true,
		},
		{
			name: "MissingUsernameWhenNoUri",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetHost("localhost").
					SetAuthSource("admin").
					SetAuthMechanism("SCRAM-SHA-256").
					SetReplicaSet("rs0").
					SetPassword("pass").
					SetTimeout(5000).
					Build()
			},
			expectError: true,
		},
		{
			name: "MissingPasswordWhenNoUri",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetHost("localhost").
					SetAuthSource("admin").
					SetAuthMechanism("SCRAM-SHA-256").
					SetReplicaSet("rs0").
					SetUsername("user").
					SetTimeout(5000).
					Build()
			},
			expectError: true,
		},
		{
			name: "MissingAuthSourceWhenNoUri",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetHost("localhost").
					SetAuthMechanism("SCRAM-SHA-256").
					SetReplicaSet("rs0").
					SetUsername("user").
					SetPassword("pass").
					SetTimeout(5000).
					Build()
			},
			expectError: true,
		},
		{
			name: "MissingTimeout",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetUri("mongodb://localhost").
					SetHost("localhost").
					SetAuthSource("admin").
					SetAuthMechanism("SCRAM-SHA-256").
					SetReplicaSet("rs0").
					SetUsername("user").
					SetPassword("pass").
					Build()
			},
			expectError: true,
		},
		{
			name: "NegativeTimeout",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetUri("mongodb://localhost").
					SetTimeout(-1).
					Build()
			},
			expectError: true,
		},
		{
			name: "ValidOptionsMinTimeout",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetUri("mongodb://localhost").
					SetTimeout(1).
					Build()
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.buildOpts()
			_, err := New(opts)
			if tt.expectError && err == nil {
				t.Errorf("expected validation error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// TestMongoOptionsBuilder tests the fluent builder pattern for MongoDB options
func TestMongoOptionsBuilder(t *testing.T) {
	t.Run("BuilderSettersChaining", func(t *testing.T) {
		opts := NewMongoOptions().
			SetUri("mongodb://localhost").
			SetHost("localhost").
			SetAuthSource("admin").
			SetAuthMechanism("SCRAM-SHA-256").
			SetReplicaSet("rs0").
			SetUsername("testuser").
			SetPassword("testpass").
			SetTimeout(5000).
			SetRetryWrites(true).
			Build()

		if opts.Uri != "mongodb://localhost" {
			t.Errorf("expected Uri to be 'mongodb://localhost', got '%s'", opts.Uri)
		}
		if opts.Host != "localhost" {
			t.Errorf("expected Host to be 'localhost', got '%s'", opts.Host)
		}
		if opts.AuthSource != "admin" {
			t.Errorf("expected AuthSource to be 'admin', got '%s'", opts.AuthSource)
		}
		if opts.AuthMechanism != "SCRAM-SHA-256" {
			t.Errorf("expected AuthMechanism to be 'SCRAM-SHA-256', got '%s'", opts.AuthMechanism)
		}
		if opts.ReplicaSet != "rs0" {
			t.Errorf("expected ReplicaSet to be 'rs0', got '%s'", opts.ReplicaSet)
		}
		if opts.Username != "testuser" {
			t.Errorf("expected Username to be 'testuser', got '%s'", opts.Username)
		}
		if opts.Password != "testpass" {
			t.Errorf("expected Password to be 'testpass', got '%s'", opts.Password)
		}
		if opts.Timeout != 5000 {
			t.Errorf("expected Timeout to be 5000, got %d", opts.Timeout)
		}
		if !opts.RetryWrites {
			t.Error("expected RetryWrites to be true")
		}
	})

	t.Run("TLSSetters", func(t *testing.T) {
		opts := NewMongoOptions().
			SetUri("mongodb://localhost").
			SetTimeout(5000).
			SetTLS(true).
			SetTLSInsecureSkipVerify(true).
			Build()

		if !opts.TLS {
			t.Error("expected TLS to be true")
		}
		if !opts.TLSInsecureSkipVerify {
			t.Error("expected TLSInsecureSkipVerify to be true")
		}
	})

	t.Run("TLSCAFileImplicitlyEnablesTLS", func(t *testing.T) {
		opts := NewMongoOptions().
			SetUri("mongodb://localhost").
			SetTimeout(5000).
			SetTLSCAFile("/etc/ssl/global-bundle.pem").
			Build()

		if opts.TLSCAFile != "/etc/ssl/global-bundle.pem" {
			t.Errorf("expected TLSCAFile to be '/etc/ssl/global-bundle.pem', got '%s'", opts.TLSCAFile)
		}
		if !opts.TLS {
			t.Error("expected TLS to be implicitly enabled when a CA file is set")
		}
	})

	t.Run("EmptyTLSCAFileDoesNotEnableTLS", func(t *testing.T) {
		opts := NewMongoOptions().
			SetUri("mongodb://localhost").
			SetTimeout(5000).
			SetTLSCAFile("").
			Build()

		if opts.TLS {
			t.Error("expected TLS to remain disabled when CA file is empty")
		}
	})

	t.Run("PartialBuilder", func(t *testing.T) {
		opts := NewMongoOptions().
			SetUri("mongodb://localhost").
			SetHost("localhost").
			Build()

		if opts.Uri != "mongodb://localhost" {
			t.Errorf("expected Uri to be set")
		}
		if opts.Host != "localhost" {
			t.Errorf("expected Host to be set")
		}
		if opts.RetryWrites {
			t.Error("expected RetryWrites to be false by default")
		}
	})
}

func TestMongoFlavorBuilder(t *testing.T) {
	t.Run("DefaultsToMongoDB", func(t *testing.T) {
		opts := NewMongoOptions().Build()

		if opts.Flavor != FlavorMongoDB {
			t.Fatalf("expected default flavor %q, got %q", FlavorMongoDB, opts.Flavor)
		}
		if opts.IsDocumentDB() {
			t.Fatal("expected the default flavor not to be DocumentDB")
		}
	})

	t.Run("NormalizesDocumentDB", func(t *testing.T) {
		opts := NewMongoOptions().SetFlavor(" DocumentDB ").Build()

		if opts.Flavor != FlavorDocumentDB {
			t.Fatalf("expected flavor %q, got %q", FlavorDocumentDB, opts.Flavor)
		}
		if !opts.IsDocumentDB() {
			t.Fatal("expected DocumentDB flavor to be detected")
		}
	})

	t.Run("RejectsUnsupportedFlavor", func(t *testing.T) {
		opts := NewMongoOptions().
			SetUri("mongodb://localhost").
			SetTimeout(5000).
			SetFlavor("unsupported").
			Build()

		if err := opts.Validate(); err == nil {
			t.Fatal("expected unsupported flavor validation to fail")
		}
	})
}

func TestMongoClientOptionsByFlavor(t *testing.T) {
	t.Run("MongoDBURIUsesStableAPIAndConfiguredRetryWrites", func(t *testing.T) {
		opts := NewMongoOptions().
			SetUri("mongodb://localhost:27017").
			SetTimeout(5000).
			SetRetryWrites(true).
			Build()

		clientOpts, err := buildMongoClientOptions(opts)
		if err != nil {
			t.Fatalf("build MongoDB client options: %v", err)
		}
		if clientOpts.ServerAPIOptions == nil {
			t.Fatal("expected MongoDB URI connection to use Stable API")
		}
		if clientOpts.RetryWrites == nil || !*clientOpts.RetryWrites {
			t.Fatal("expected MongoDB connection to preserve retryWrites=true")
		}
	})

	t.Run("DocumentDBDisablesStableAPIAndRetryWrites", func(t *testing.T) {
		opts := NewMongoOptions().
			SetUri("mongodb://documentdb.example:27017").
			SetTimeout(5000).
			SetFlavor(FlavorDocumentDB).
			SetRetryWrites(true).
			Build()

		clientOpts, err := buildMongoClientOptions(opts)
		if err != nil {
			t.Fatalf("build DocumentDB client options: %v", err)
		}
		if clientOpts.ServerAPIOptions != nil {
			t.Fatal("expected DocumentDB connection not to request MongoDB Stable API")
		}
		if clientOpts.RetryWrites == nil || *clientOpts.RetryWrites {
			t.Fatal("expected DocumentDB connection to force retryWrites=false")
		}
	})

	t.Run("DocumentDBComponentsDefaultToSCRAMSHA1", func(t *testing.T) {
		opts := NewMongoOptions().
			SetHost("documentdb.example:27017").
			SetAuthSource("admin").
			SetUsername("user").
			SetPassword("password").
			SetTimeout(5000).
			SetFlavor(FlavorDocumentDB).
			Build()

		clientOpts, err := buildMongoClientOptions(opts)
		if err != nil {
			t.Fatalf("build DocumentDB client options: %v", err)
		}
		if clientOpts.Auth == nil || clientOpts.Auth.AuthMechanism != "SCRAM-SHA-1" {
			t.Fatalf("expected DocumentDB auth mechanism SCRAM-SHA-1, got %#v", clientOpts.Auth)
		}
		if opts.AuthMechanism != "SCRAM-SHA-1" {
			t.Fatalf("expected resolved auth mechanism to be stored in options, got %q", opts.AuthMechanism)
		}
	})
}

func TestMongoClientOptionsTLS(t *testing.T) {
	t.Run("DisabledByDefault", func(t *testing.T) {
		opts := NewMongoOptions().
			SetUri("mongodb://localhost:27017").
			SetTimeout(5000).
			Build()

		clientOpts, err := buildMongoClientOptions(opts)
		if err != nil {
			t.Fatalf("build client options: %v", err)
		}
		if clientOpts.TLSConfig != nil {
			t.Fatal("expected TLS configuration to remain unset")
		}
	})

	t.Run("EnforcesTLS12AndExplicitInsecureMode", func(t *testing.T) {
		opts := NewMongoOptions().
			SetUri("mongodb://localhost:27017").
			SetTimeout(5000).
			SetTLS(true).
			SetTLSInsecureSkipVerify(true).
			Build()

		clientOpts, err := buildMongoClientOptions(opts)
		if err != nil {
			t.Fatalf("build client options: %v", err)
		}
		if clientOpts.TLSConfig == nil {
			t.Fatal("expected TLS configuration")
		}
		if clientOpts.TLSConfig.MinVersion != tls.VersionTLS12 {
			t.Fatalf("expected minimum TLS version 1.2, got %d", clientOpts.TLSConfig.MinVersion)
		}
		if !clientOpts.TLSConfig.InsecureSkipVerify {
			t.Fatal("expected explicitly configured insecure verification mode")
		}
	})

	t.Run("LoadsCustomCAAndImplicitlyEnablesTLS", func(t *testing.T) {
		server := httptest.NewTLSServer(nil)
		defer server.Close()

		caFile := filepath.Join(t.TempDir(), "ca.pem")
		certificate := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: server.Certificate().Raw,
		})
		if err := os.WriteFile(caFile, certificate, 0o600); err != nil {
			t.Fatalf("write test CA: %v", err)
		}

		opts := NewMongoOptions().
			SetUri("mongodb://localhost:27017").
			SetTimeout(5000).
			SetTLSCAFile(caFile).
			Build()

		clientOpts, err := buildMongoClientOptions(opts)
		if err != nil {
			t.Fatalf("build client options: %v", err)
		}
		if clientOpts.TLSConfig == nil || clientOpts.TLSConfig.RootCAs == nil {
			t.Fatal("expected custom CA pool to be injected into TLS configuration")
		}
		if len(clientOpts.TLSConfig.RootCAs.Subjects()) != 1 {
			t.Fatalf("expected one custom CA, got %d", len(clientOpts.TLSConfig.RootCAs.Subjects()))
		}
	})

	t.Run("RejectsMissingCAFile", func(t *testing.T) {
		caFile := filepath.Join(t.TempDir(), "missing.pem")
		opts := NewMongoOptions().
			SetUri("mongodb://localhost:27017").
			SetTimeout(5000).
			SetTLSCAFile(caFile).
			Build()

		_, err := buildMongoClientOptions(opts)
		if err == nil || !strings.Contains(err.Error(), "failed to read TLS CA file") {
			t.Fatalf("expected missing CA error, got %v", err)
		}
	})

	t.Run("RejectsMalformedCAFile", func(t *testing.T) {
		caFile := filepath.Join(t.TempDir(), "invalid.pem")
		if err := os.WriteFile(caFile, []byte("not a certificate"), 0o600); err != nil {
			t.Fatalf("write malformed CA: %v", err)
		}
		opts := NewMongoOptions().
			SetUri("mongodb://localhost:27017").
			SetTimeout(5000).
			SetTLSCAFile(caFile).
			Build()

		_, err := buildMongoClientOptions(opts)
		if err == nil || !strings.Contains(err.Error(), "failed to parse any certificates") {
			t.Fatalf("expected malformed CA error, got %v", err)
		}
	})
}

func TestMongodbLiveIntegration(t *testing.T) {

	tests := []struct {
		name        string
		buildOpts   func() *MongoOptions
		expectError bool
	}{
		{
			name: "UriIntegrationTest",
			buildOpts: func() *MongoOptions {
				mongodbUri := os.Getenv("MONGODB_URI")
				return NewMongoOptions().
					SetUri(mongodbUri).
					SetTimeout(5000).
					Build()
			},
			expectError: false,
		},
		{
			name: "ComponentsIntegrationTest",
			buildOpts: func() *MongoOptions {
				mongodbHost := os.Getenv("MONGODB_HOST")
				mongodbAuthSource := os.Getenv("MONGODB_DATABASE_CREDENTIALS")
				mongodbUsername := os.Getenv("MONGODB_USERNAME")
				mongodbPassword := os.Getenv("MONGODB_PASSWORD")

				return NewMongoOptions().
					SetHost(mongodbHost).
					SetAuthSource(mongodbAuthSource).
					SetUsername(mongodbUsername).
					SetPassword(mongodbPassword).
					SetTimeout(5000).
					Build()
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.name {
			case "UriIntegrationTest":
				if !envVarsSet("MONGODB_URI") {
					t.Skip("MONGODB_URI not set, skipping integration test")
				}
			case "ComponentsIntegrationTest":
				if !envVarsSet("MONGODB_HOST", "MONGODB_DATABASE_CREDENTIALS", "MONGODB_USERNAME", "MONGODB_PASSWORD") {
					t.Skip("MongoDB component environment variables not set, skipping integration test")
				}
			}

			opts := tt.buildOpts()
			db, err := New(opts)
			if err != nil {
				t.Fatalf("failed to create database instance: %v", err)
			}

			options := db.Options.(*MongoOptions)

			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(options.Timeout)*time.Millisecond)
			defer cancel()

			err = db.Client.Ping(ctx)
			if tt.expectError && err == nil {
				t.Errorf("expected ping error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no ping error but got: %v", err)
			}
		})
	}
}

func TestFindIntegration(t *testing.T) {
	mongodbUri := os.Getenv("MONGODB_URI")
	if mongodbUri == "" {
		t.Skip("MONGODB_URI not set, skipping integration test")
	}

	opts := NewMongoOptions().
		SetUri(mongodbUri).
		SetTimeout(5000).
		Build()

	db, err := New(opts)
	if err != nil {
		t.Fatalf("failed to create database instance: %v", err)
	}

	options := db.Options.(*MongoOptions)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(options.Timeout)*time.Millisecond)
	defer cancel()

	// Test Find with username filter
	filter := map[string]any{"username": "cedricve"}
	var resultSlice []any
	err = db.Client.Find(ctx, "Kerberos", "users", filter).All(&resultSlice)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if len(resultSlice) != 1 {
		t.Fatalf("expected exactly 1 result for username 'cedricve', got %d", len(resultSlice))
	}

	// Marshal the result to User struct
	resultBytes, err := bson.Marshal(resultSlice[0])
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	var user models.User
	err = bson.Unmarshal(resultBytes, &user)
	if err != nil {
		t.Fatalf("failed to unmarshal to User struct: %v", err)
	}

	// Validate user fields
	if user.Username != "cedricve" {
		t.Errorf("expected username 'cedricve', got '%s'", user.Username)
	}
}
