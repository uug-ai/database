package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/uug-ai/models/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
)

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
					SetTimeout(2000).
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
			opts := tt.buildOpts()
			db, err := New(opts)
			if err != nil {
				t.Fatalf("failed to create database instance: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(db.Options.Timeout)*time.Millisecond)
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
		SetTimeout(1000).
		Build()

	db, err := New(opts)
	if err != nil {
		t.Fatalf("failed to create database instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(db.Options.Timeout)*time.Millisecond)
	defer cancel()

	// Test Find with username filter
	filter := map[string]interface{}{"username": "cedricve"}
	results, err := db.Client.Find(ctx, "Kerberos", "users", filter)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	// Validate results
	resultSlice, ok := results.([]interface{})
	if !ok {
		t.Fatalf("expected results to be []interface{}, got %T", results)
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
