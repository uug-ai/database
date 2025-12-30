# Database

Universal MongoDB database driver for Go with built-in observability and functional options pattern.

[![Go Version](https://img.shields.io/badge/Go-1.24-blue.svg)](https://go.dev/) [![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE) [![Go Report Card](https://goreportcard.com/badge/github.com/uug-ai/database)](https://goreportcard.com/report/github.com/uug-ai/database)

A Go library for connecting to and managing MongoDB with a unified interface using the functional options builder pattern. Includes built-in OpenTelemetry instrumentation for comprehensive observability.

## Features

- **MongoDB Support**: Full MongoDB driver integration with connection pooling
- **Options Builder Pattern**: Clean, fluent interface for configuration
- **Built-in Validation**: Compile-time type safety with validation
- **OpenTelemetry Integration**: Distributed tracing and observability out of the box
- **Context Support**: Full context.Context support for timeouts and cancellation
- **Comprehensive Tests**: Full test coverage including mocks
- **Production Ready**: Optimized for high-performance applications

## Installation

```bash
go get github.com/uug-ai/database
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    "time"
    "github.com/uug-ai/database/pkg/database"
)

func main() {
    // Build MongoDB options
    opts := database.NewMongoOptions().
        SetUri("mongodb://localhost:27017").
        SetHost("localhost").
        SetAuthSource("admin").
        SetAuthMechanism("SCRAM-SHA-256").
        SetReplicaSet("rs0").
        SetUsername("user").
        SetPassword("password").
        SetTimeout(10).
        Build()

    // Create database client with options
    db, err := database.New(opts)
    if err != nil {
        log.Fatal(err)
    }

    // Ping the database to verify connection
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := db.Ping(ctx); err != nil {
        log.Fatal(err)
    }

    log.Println("Successfully connected to MongoDB!")
}
```

## Core Concepts

### Options Builder Pattern

All components use the options builder pattern (similar to MongoDB's official driver). This provides:

- **Clean Syntax**: Build options separately, then pass to constructor
- **Readability**: Self-documenting method chains
- **Separation of Concerns**: Options building is separate from client creation
- **Validation**: Built-in validation when creating the client
- **Type Safety**: Compile-time type checking
- **Flexibility**: Configure only what you need

### Creating a Database Client

Each database connection follows this pattern:

1. Build Options using `database.NewMongoOptions()` with method chaining
2. Call `.Build()` to get the options object
3. Create Client by passing options to `database.New(opts)`
4. Use the client for database operations

## Usage Examples

### MongoDB Connection

The MongoDB integration demonstrates the options builder pattern:

```go
package main

import (
    "context"
    "log"
    "time"
    "github.com/uug-ai/database/pkg/database"
)

func main() {
    // Build MongoDB options
    opts := database.NewMongoOptions().
        SetUri("mongodb+srv://user:password@cluster.mongodb.net/?retryWrites=true&w=majority").
        SetHost("cluster.mongodb.net").
        SetAuthSource("admin").
        SetAuthMechanism("SCRAM-SHA-256").
        SetUsername("user").
        SetPassword("password").
        SetTimeout(30).
        SetRetryWrites(true).
        Build()

    // Create database client with options
    db, err := database.New(opts)
    if err != nil {
        log.Fatal(err)
    }

    // Perform database operations
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    err = db.Ping(ctx)
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Connected to MongoDB successfully!")
}
```

**Available Methods:**

- `.SetUri(uri string)` - MongoDB connection URI
- `.SetHost(host string)` - Database host address
- `.SetAuthSource(source string)` - Authentication source database
- `.SetAuthMechanism(mechanism string)` - Authentication mechanism (e.g., SCRAM-SHA-256)
- `.SetReplicaSet(replicaSet string)` - Replica set name
- `.SetUsername(username string)` - Database username
- `.SetPassword(password string)` - Database password
- `.SetTimeout(seconds int)` - Connection timeout in seconds
- `.SetRetryWrites(retry bool)` - Enable automatic retry writes
- `.Build()` - Returns the MongoOptions object

## Project Structure

```
.
├── pkg/
│   └── database/              # Core database implementation
│       ├── database.go        # Main Database struct
│       ├── mongodb.go         # MongoDB client implementation
│       ├── mongodb_test.go    # MongoDB tests
│       └── option.go          # Functional option types
├── main.go
├── go.mod
├── go.sum
├── Dockerfile
└── README.md
```

## Configuration

### Using the Options Builder Pattern (Recommended)

```go
opts := database.NewMongoOptions().
    SetUri("mongodb://localhost:27017").
    SetHost("localhost").
    SetAuthSource("admin").
    SetAuthMechanism("SCRAM-SHA-256").
    SetReplicaSet("rs0").
    SetUsername("admin").
    SetPassword("password").
    SetTimeout(10).
    Build()

db, err := database.New(opts)
```

### Environment Variables

You can load configuration from environment variables:

```go
import "os"

opts := database.NewMongoOptions().
    SetUri(os.Getenv("MONGO_URI")).
    SetHost(os.Getenv("MONGO_HOST")).
    SetAuthSource(os.Getenv("MONGO_AUTH_SOURCE")).
    SetAuthMechanism(os.Getenv("MONGO_AUTH_MECHANISM")).
    SetReplicaSet(os.Getenv("MONGO_REPLICA_SET")).
    SetUsername(os.Getenv("MONGO_USERNAME")).
    SetPassword(os.Getenv("MONGO_PASSWORD")).
    SetTimeout(30).
    Build()

db, err := database.New(opts)
```

**Example `.env` file:**

```env
# MongoDB Configuration
MONGO_URI=mongodb://localhost:27017
MONGO_HOST=localhost
MONGO_AUTH_SOURCE=admin
MONGO_AUTH_MECHANISM=SCRAM-SHA-256
MONGO_REPLICA_SET=rs0
MONGO_USERNAME=admin
MONGO_PASSWORD=password
```

## Validation

MongoDB options use [go-playground/validator](https://github.com/go-playground/validator) for configuration validation. All required fields must be provided:

- `Uri` - Connection URI (required)
- `Host` - Database host (required)
- `AuthSource` - Auth source database (required)
- `AuthMechanism` - Auth mechanism type (required)
- `ReplicaSet` - Replica set name (required)
- `Username` - Database username (required)
- `Password` - Database password (required)
- `Timeout` - Connection timeout >= 0 (required)

Validation is automatically performed when calling `database.New(opts)`, ensuring invalid configurations are caught before the client is created.

## Error Handling

The options builder pattern provides clear error handling:

```go
// Build options (no validation here)
opts := database.NewMongoOptions().
    SetUri("mongodb://localhost:27017").
    SetHost("localhost").
    // Missing required fields...
    Build()

// Validation happens when creating the client
db, err := database.New(opts)
if err != nil {
    // Validation error caught at client creation time
    log.Printf("Configuration error: %v", err)
    return
}

// If we get here, the configuration is valid
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err = db.Ping(ctx)
if err != nil {
    // Runtime error during operation
    log.Printf("Connection error: %v", err)
    return
}
```

## Testing

Run the test suite:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test -cover ./...
```

Run tests for specific components:

```bash
# Database tests
go test ./pkg/database -v

# MongoDB tests
go test ./pkg/database -run TestMongo
```

## OpenTelemetry Integration

This package includes built-in OpenTelemetry instrumentation for MongoDB operations:

```go
import (
    "github.com/uug-ai/database/pkg/database"
    "go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
)

// OpenTelemetry is automatically configured in NewMongoClient
opts := database.NewMongoOptions().
    SetUri("mongodb://localhost:27017").
    // ... other options
    Build()

db, err := database.New(opts)
// Automatic tracing enabled!
```

## Contributing

Contributions are welcome! When adding new features or database drivers, please follow the options builder pattern demonstrated in this repository.

### Development Guidelines

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/amazing-feature`)
3. Follow the options builder pattern
4. Add comprehensive tests for your changes
5. Ensure all tests pass: `go test ./...`
6. Commit your changes following [Conventional Commits](https://www.conventionalcommits.org/)
7. Push to your branch (`git push origin feat/amazing-feature`)
8. Open a Pull Request

### Commit Message Format

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

**Types:** `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `types`

**Scopes:**

- `database` - Core database functionality
- `mongo` - MongoDB driver
- `options` - Options builder
- `docs` - Documentation updates
- `tests` - Test updates

**Examples:**

```
feat(mongo): add connection pooling configuration
fix(database): correct context timeout handling
docs(readme): update MongoDB connection examples
refactor(options): simplify builder interface
test(mongo): add replica set failover tests
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Dependencies

This project uses the following key libraries:

- [go-playground/validator](https://github.com/go-playground/validator) - Struct validation
- [mongo-driver](https://github.com/mongodb/mongo-go-driver) - Official MongoDB Go driver
- [OpenTelemetry](https://opentelemetry.io/) - Observability and tracing

See [go.mod](go.mod) for the complete list of dependencies.

## Benefits of the Options Builder Pattern

### Clean Syntax

Build options separately from client creation:

```go
opts := database.NewMongoOptions().
    SetUri("mongodb://localhost:27017").
    SetHost("localhost").
    Build()

db, err := database.New(opts)
```

### Separation of Concerns

Options building is completely separate from client creation, following the same pattern as MongoDB's official driver.

### Type Safety

Compile-time type checking prevents configuration errors.

### Flexibility

Configure only the options you need. Method chaining is optional.

### Validation

Built-in validation when creating the client ensures configurations are correct before use, catching errors early.

### Extensibility

Adding new builder methods doesn't break existing code. Simply add new chainable methods to the options builder.

### Readability

Self-documenting fluent API makes code easy to read and understand:

```go
// Clear and readable - MongoDB style
opts := database.NewMongoOptions().
    SetUri("mongodb+srv://user:password@cluster.mongodb.net").
    SetHost("cluster.mongodb.net").
    SetAuthSource("admin").
    SetAuthMechanism("SCRAM-SHA-256").
    SetUsername("user").
    SetPassword("password").
    SetTimeout(30).
    Build()

db, err := database.New(opts)
```

## Support

- **Issues**: [GitHub Issues](https://github.com/uug-ai/database/issues)
- **Discussions**: [GitHub Discussions](https://github.com/uug-ai/database/discussions)
- **Documentation**: See inline code comments and examples above