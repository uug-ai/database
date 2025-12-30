package database

import (
	"testing"
)

// MockDatabaseInterface is a mock implementation of DatabaseInterface for testing
type MockDatabaseInterface struct {
	PingCalled bool
	PingError  error
}

func (m *MockDatabaseInterface) Ping() error {
	m.PingCalled = true
	return m.PingError
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
			name: "ValidOptionsWithComponents",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetUri("mongodb://placeholder").
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
			name: "MissingUri",
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
			expectError: true,
		},
		{
			name: "MissingHost",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetUri("mongodb://localhost").
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
			name: "MissingUsername",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetUri("mongodb://localhost").
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
			name: "MissingPassword",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetUri("mongodb://localhost").
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
			name: "MissingAuthSource",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetUri("mongodb://localhost").
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
			name: "MissingAuthMechanism",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetUri("mongodb://localhost").
					SetHost("localhost").
					SetAuthSource("admin").
					SetReplicaSet("rs0").
					SetUsername("user").
					SetPassword("pass").
					SetTimeout(5000).
					Build()
			},
			expectError: true,
		},
		{
			name: "MissingReplicaSet",
			buildOpts: func() *MongoOptions {
				return NewMongoOptions().
					SetUri("mongodb://localhost").
					SetHost("localhost").
					SetAuthSource("admin").
					SetAuthMechanism("SCRAM-SHA-256").
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
					SetHost("localhost").
					SetAuthSource("admin").
					SetAuthMechanism("SCRAM-SHA-256").
					SetReplicaSet("rs0").
					SetUsername("user").
					SetPassword("pass").
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
					SetHost("localhost").
					SetAuthSource("admin").
					SetAuthMechanism("SCRAM-SHA-256").
					SetReplicaSet("rs0").
					SetUsername("user").
					SetPassword("pass").
					SetTimeout(1).
					Build()
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.buildOpts()
			mockClient := &MockDatabaseInterface{}

			_, err := New(opts, mockClient)

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
