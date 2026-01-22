package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// MockDatabase is a mock implementation of DatabaseInterface for testing
type MockDatabase struct {
	// PingFunc allows customizing Ping behavior
	PingFunc func(ctx context.Context) error

	// FindFunc allows customizing Find behavior
	FindFunc func(ctx context.Context, db string, collection string, filter any, opts ...any) (any, error)

	// FindOneFunc allows customizing FindOne behavior - returns a SingleResultInterface
	FindOneFunc func(ctx context.Context, db string, collection string, filter any, opts ...any) SingleResultInterface

	// Sequential response queues for multiple calls
	PingQueue    []PingResponse
	FindQueue    []FindResponse
	FindOneQueue []FindOneResponse

	// Call tracking
	PingCalls    []PingCall
	FindCalls    []FindCall
	FindOneCalls []FindOneCall
}

// MockSingleResult implements SingleResultInterface for testing
type MockSingleResult struct {
	result any
	err    error
}

// Into decodes the result into dest
func (m *MockSingleResult) Into(dest any) error {
	if m.err != nil {
		return m.err
	}
	if m.result == nil {
		return fmt.Errorf("no document found")
	}
	return copyResult(m.result, dest)
}

// Raw returns the raw result
func (m *MockSingleResult) Raw() (any, error) {
	return m.result, m.err
}

// Err returns any error
func (m *MockSingleResult) Err() error {
	return m.err
}

// PingResponse represents a queued response for Ping
type PingResponse struct {
	Err error
}

// FindResponse represents a queued response for Find
type FindResponse struct {
	Result any
	Err    error
}

// FindOneResponse represents a queued response for FindOne
type FindOneResponse struct {
	Result any
	Err    error
}

// PingCall records a call to Ping
type PingCall struct {
	Ctx context.Context
}

// FindCall records a call to Find
type FindCall struct {
	Ctx        context.Context
	Db         string
	Collection string
	Filter     any
	Opts       []any
}

// FindOneCall records a call to FindOne
type FindOneCall struct {
	Ctx        context.Context
	Db         string
	Collection string
	Filter     any
	Opts       []any
}

// NewMockDatabase creates a new MockDatabase with sensible defaults
func NewMockDatabase() *MockDatabase {
	return &MockDatabase{
		PingFunc: func(ctx context.Context) error {
			return nil
		},
		FindFunc: func(ctx context.Context, db string, collection string, filter any, opts ...any) (any, error) {
			return []any{}, nil
		},
		FindOneFunc: func(ctx context.Context, db string, collection string, filter any, opts ...any) SingleResultInterface {
			return &MockSingleResult{result: nil, err: fmt.Errorf("no document found")}
		},
		PingCalls:    []PingCall{},
		FindCalls:    []FindCall{},
		FindOneCalls: []FindOneCall{},
		PingQueue:    []PingResponse{},
		FindQueue:    []FindResponse{},
		FindOneQueue: []FindOneResponse{},
	}
}

// Ping implements DatabaseInterface
func (m *MockDatabase) Ping(ctx context.Context) error {
	m.PingCalls = append(m.PingCalls, PingCall{Ctx: ctx})

	// Check if there's a queued response
	if len(m.PingQueue) > 0 {
		response := m.PingQueue[0]
		m.PingQueue = m.PingQueue[1:]
		return response.Err
	}

	// Fall back to PingFunc
	if m.PingFunc != nil {
		return m.PingFunc(ctx)
	}
	return nil
}

// Disconnect implements DatabaseInterface
func (m *MockDatabase) Disconnect(ctx context.Context) error {
	return nil
}

// GetTimeout returns the timeout duration
func (m *MockDatabase) GetTimeout() time.Duration {
	return 5 * time.Second
}

// Find implements DatabaseInterface
func (m *MockDatabase) Find(ctx context.Context, db string, collection string, filter any, opts ...any) (any, error) {
	m.FindCalls = append(m.FindCalls, FindCall{
		Ctx:        ctx,
		Db:         db,
		Collection: collection,
		Filter:     filter,
		Opts:       opts,
	})

	// Check if there's a queued response
	if len(m.FindQueue) > 0 {
		response := m.FindQueue[0]
		m.FindQueue = m.FindQueue[1:]
		return response.Result, response.Err
	}

	// Fall back to FindFunc
	if m.FindFunc != nil {
		return m.FindFunc(ctx, db, collection, filter, opts...)
	}
	return []any{}, nil
}

// FindOne implements DatabaseInterface
func (m *MockDatabase) FindOne(ctx context.Context, db string, collection string, filter any, opts ...any) SingleResultInterface {
	m.FindOneCalls = append(m.FindOneCalls, FindOneCall{
		Ctx:        ctx,
		Db:         db,
		Collection: collection,
		Filter:     filter,
		Opts:       opts,
	})

	// Check if there's a queued response
	if len(m.FindOneQueue) > 0 {
		response := m.FindOneQueue[0]
		m.FindOneQueue = m.FindOneQueue[1:]
		return &MockSingleResult{result: response.Result, err: response.Err}
	}

	// Fall back to FindOneFunc
	if m.FindOneFunc != nil {
		return m.FindOneFunc(ctx, db, collection, filter, opts...)
	}
	return &MockSingleResult{result: nil, err: fmt.Errorf("no document found")}
}

// copyResult copies src into dest using BSON marshaling (for mock testing)
func copyResult(src any, dest any) error {
	bytes, err := bson.Marshal(src)
	if err != nil {
		return err
	}
	return bson.Unmarshal(bytes, dest)
}

// InsertOne implements DatabaseInterface
func (m *MockDatabase) InsertOne(ctx context.Context, db string, collection string, document any, opts ...any) (any, error) {
	return nil, fmt.Errorf("InsertOne not implemented in MockDatabase")
}

// InsertMany implements DatabaseInterface
func (m *MockDatabase) InsertMany(ctx context.Context, db string, collection string, documents []any, opts ...any) (any, error) {
	return nil, fmt.Errorf("InsertMany not implemented in MockDatabase")
}

// Reset clears all recorded calls
func (m *MockDatabase) Reset() {
	m.PingCalls = []PingCall{}
	m.FindCalls = []FindCall{}
	m.FindOneCalls = []FindOneCall{}
	m.PingQueue = []PingResponse{}
	m.FindQueue = []FindResponse{}
	m.FindOneQueue = []FindOneResponse{}
}

// ExpectPing sets up an expectation for Ping
func (m *MockDatabase) ExpectPing(err error) *MockDatabase {
	m.PingFunc = func(ctx context.Context) error {
		return err
	}
	return m
}

// ExpectFind sets up an expectation for Find
func (m *MockDatabase) ExpectFind(result any, err error) *MockDatabase {
	m.FindFunc = func(ctx context.Context, db string, collection string, filter any, opts ...any) (any, error) {
		return result, err
	}
	return m
}

// ExpectFindOne sets up an expectation for FindOne
func (m *MockDatabase) ExpectFindOne(result any, err error) *MockDatabase {
	m.FindOneFunc = func(ctx context.Context, db string, collection string, filter any, opts ...any) SingleResultInterface {
		return &MockSingleResult{result: result, err: err}
	}
	return m
}

// QueuePing adds a Ping response to the queue for sequential calls
func (m *MockDatabase) QueuePing(err error) *MockDatabase {
	m.PingQueue = append(m.PingQueue, PingResponse{Err: err})
	return m
}

// QueueFind adds a Find response to the queue for sequential calls
func (m *MockDatabase) QueueFind(result any, err error) *MockDatabase {
	m.FindQueue = append(m.FindQueue, FindResponse{Result: result, Err: err})
	return m
}

// QueueFindOne adds a FindOne response to the queue for sequential calls
func (m *MockDatabase) QueueFindOne(result any, err error) *MockDatabase {
	m.FindOneQueue = append(m.FindOneQueue, FindOneResponse{Result: result, Err: err})
	return m
}
