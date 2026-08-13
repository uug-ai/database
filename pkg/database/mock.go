package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// MockDatabase is a mock implementation of DatabaseInterface for testing
type MockDatabase struct {
	// PingFunc allows customizing Ping behavior
	PingFunc func(ctx context.Context) error

	// FindFunc allows customizing Find behavior - returns a FindResultInterface
	FindFunc func(ctx context.Context, db string, collection string, filter any, opts ...any) FindResultInterface

	// FindOneFunc allows customizing FindOne behavior - returns a SingleResultInterface
	FindOneFunc func(ctx context.Context, db string, collection string, filter any, opts ...any) SingleResultInterface

	// FindOneAndUpdateFunc allows customizing atomic find-and-update behavior.
	FindOneAndUpdateFunc func(ctx context.Context, db string, collection string, filter any, update any, opts ...any) SingleResultInterface

	// UpdateOneFunc allows customizing UpdateOne behavior
	UpdateOneFunc func(ctx context.Context, db string, collection string, filter any, update any, opts ...any) (UpdateResultInterface, error)

	// DeleteOneFunc allows customizing DeleteOne behavior
	DeleteOneFunc func(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error)

	// DeleteManyFunc allows customizing DeleteMany behavior
	DeleteManyFunc func(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error)

	// CountFunc allows customizing Count behavior
	CountFunc func(ctx context.Context, db string, collection string, filter any, opts ...any) (int64, error)

	// Sequential response queues for multiple calls
	PingQueue             []PingResponse
	FindQueue             []FindResponse
	FindOneQueue          []FindOneResponse
	FindOneAndUpdateQueue []FindOneResponse
	UpdateOneQueue        []UpdateOneResponse
	DeleteOneQueue        []DeleteResponse
	DeleteManyQueue       []DeleteResponse
	CountQueue            []CountResponse

	// Call tracking
	PingCalls             []PingCall
	FindCalls             []FindCall
	FindOneCalls          []FindOneCall
	FindOneAndUpdateCalls []FindOneAndUpdateCall
	UpdateOneCalls        []UpdateOneCall
	DeleteOneCalls        []DeleteCall
	DeleteManyCalls       []DeleteCall
	CountCalls            []CountCall
}

var _ DatabaseInterface = (*MockDatabase)(nil)

// MockSingleResult implements SingleResultInterface for testing
type MockSingleResult struct {
	result any
	err    error
}

// MockUpdateResult implements UpdateResultInterface for testing
type MockUpdateResult struct {
	matchedCount  int64
	modifiedCount int64
	upsertedCount int64
	upsertedID    any
}

// MockDeleteResult implements DeleteResultInterface for testing
type MockDeleteResult struct {
	deletedCount int64
}

// MockFindResult implements FindResultInterface for testing
type MockFindResult struct {
	results any
	err     error
}

// All decodes all results into dest
func (m *MockFindResult) All(dest any) error {
	if m.err != nil {
		return m.err
	}
	if m.results == nil {
		return nil
	}
	return copySliceResult(m.results, dest)
}

// Err returns any error
func (m *MockFindResult) Err() error {
	return m.err
}

// MatchedCount returns the number of documents matched
func (m *MockUpdateResult) MatchedCount() int64 {
	return m.matchedCount
}

// ModifiedCount returns the number of documents modified
func (m *MockUpdateResult) ModifiedCount() int64 {
	return m.modifiedCount
}

// UpsertedCount returns the number of documents upserted
func (m *MockUpdateResult) UpsertedCount() int64 {
	return m.upsertedCount
}

// UpsertedID returns the ID of the upserted document
func (m *MockUpdateResult) UpsertedID() any {
	return m.upsertedID
}

// DeletedCount returns the number of documents deleted
func (m *MockDeleteResult) DeletedCount() int64 {
	return m.deletedCount
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

// UpdateOneResponse represents a queued response for UpdateOne
type UpdateOneResponse struct {
	MatchedCount  int64
	ModifiedCount int64
	UpsertedCount int64
	UpsertedID    any
	Err           error
}

// DeleteResponse represents a queued response for delete operations
type DeleteResponse struct {
	DeletedCount int64
	Err          error
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

// FindOneAndUpdateCall records an atomic find-and-update call.
type FindOneAndUpdateCall struct {
	Ctx        context.Context
	Db         string
	Collection string
	Filter     any
	Update     any
	Opts       []any
}

// UpdateOneCall records a call to UpdateOne
type UpdateOneCall struct {
	Ctx        context.Context
	Db         string
	Collection string
	Filter     any
	Update     any
	Opts       []any
}

// DeleteCall records a call to DeleteOne or DeleteMany
type DeleteCall struct {
	Ctx        context.Context
	Db         string
	Collection string
	Filter     any
	Opts       []any
}

// CountResponse represents a queued response for Count
type CountResponse struct {
	Count int64
	Err   error
}

// CountCall records a call to Count
type CountCall struct {
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
		FindFunc: func(ctx context.Context, db string, collection string, filter any, opts ...any) FindResultInterface {
			return &MockFindResult{results: []any{}, err: nil}
		},
		FindOneFunc: func(ctx context.Context, db string, collection string, filter any, opts ...any) SingleResultInterface {
			return &MockSingleResult{result: nil, err: fmt.Errorf("no document found")}
		},
		FindOneAndUpdateFunc: func(ctx context.Context, db string, collection string, filter any, update any, opts ...any) SingleResultInterface {
			return &MockSingleResult{result: nil, err: fmt.Errorf("no document found")}
		},
		UpdateOneFunc: func(ctx context.Context, db string, collection string, filter any, update any, opts ...any) (UpdateResultInterface, error) {
			return &MockUpdateResult{matchedCount: 1, modifiedCount: 1}, nil
		},
		DeleteOneFunc: func(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error) {
			return &MockDeleteResult{deletedCount: 1}, nil
		},
		DeleteManyFunc: func(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error) {
			return &MockDeleteResult{deletedCount: 0}, nil
		},
		CountFunc: func(ctx context.Context, db string, collection string, filter any, opts ...any) (int64, error) {
			return 0, nil
		},
		PingCalls:             []PingCall{},
		FindCalls:             []FindCall{},
		FindOneCalls:          []FindOneCall{},
		FindOneAndUpdateCalls: []FindOneAndUpdateCall{},
		UpdateOneCalls:        []UpdateOneCall{},
		DeleteOneCalls:        []DeleteCall{},
		DeleteManyCalls:       []DeleteCall{},
		CountCalls:            []CountCall{},
		PingQueue:             []PingResponse{},
		FindQueue:             []FindResponse{},
		FindOneQueue:          []FindOneResponse{},
		FindOneAndUpdateQueue: []FindOneResponse{},
		UpdateOneQueue:        []UpdateOneResponse{},
		DeleteOneQueue:        []DeleteResponse{},
		DeleteManyQueue:       []DeleteResponse{},
		CountQueue:            []CountResponse{},
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
func (m *MockDatabase) Find(ctx context.Context, db string, collection string, filter any, opts ...any) FindResultInterface {
	m.FindCalls = append(m.FindCalls, FindCall{
		Ctx:        ctx,
		Db:         db,
		Collection: collection,
		Filter:     filter,
		Opts:       opts,
	})

	var result any
	var err error

	// Check if there's a queued response
	if len(m.FindQueue) > 0 {
		response := m.FindQueue[0]
		m.FindQueue = m.FindQueue[1:]
		result = response.Result
		err = response.Err
	} else if m.FindFunc != nil {
		// Fall back to FindFunc
		return m.FindFunc(ctx, db, collection, filter, opts...)
	} else {
		result = []any{}
		err = nil
	}

	// Apply projection if present
	if result != nil && err == nil {
		for _, opt := range opts {
			if proj, ok := opt.(*Projection); ok {
				result = applyProjectionToSlice(result, proj)
				break
			}
		}
	}

	return &MockFindResult{results: result, err: err}
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

	var result any
	var err error

	// Check if there's a queued response
	if len(m.FindOneQueue) > 0 {
		response := m.FindOneQueue[0]
		m.FindOneQueue = m.FindOneQueue[1:]
		result = response.Result
		err = response.Err
	} else if m.FindOneFunc != nil {
		// Fall back to FindOneFunc
		mockResult := m.FindOneFunc(ctx, db, collection, filter, opts...)
		result, err = mockResult.Raw()
	} else {
		result = nil
		err = fmt.Errorf("no document found")
	}

	// Apply projection if present
	if result != nil && err == nil {
		for _, opt := range opts {
			if proj, ok := opt.(*Projection); ok {
				result = applyProjection(result, proj)
				break
			}
		}
	}

	return &MockSingleResult{result: result, err: err}
}

// FindOneAndUpdate implements DatabaseInterface.
func (m *MockDatabase) FindOneAndUpdate(ctx context.Context, db string, collection string, filter any, update any, opts ...any) SingleResultInterface {
	m.FindOneAndUpdateCalls = append(m.FindOneAndUpdateCalls, FindOneAndUpdateCall{
		Ctx:        ctx,
		Db:         db,
		Collection: collection,
		Filter:     filter,
		Update:     update,
		Opts:       opts,
	})

	if len(m.FindOneAndUpdateQueue) > 0 {
		response := m.FindOneAndUpdateQueue[0]
		m.FindOneAndUpdateQueue = m.FindOneAndUpdateQueue[1:]
		return &MockSingleResult{result: response.Result, err: response.Err}
	}
	if m.FindOneAndUpdateFunc != nil {
		return m.FindOneAndUpdateFunc(ctx, db, collection, filter, update, opts...)
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

// copySliceResult copies a slice from src into dest using JSON marshaling
// This is simpler than BSON for arrays at the top level
func copySliceResult(src any, dest any) error {
	// Use standard JSON which handles arrays at top level correctly
	bytes, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, dest)
}

// applyProjection filters fields from a result based on projection rules
func applyProjection(result any, proj *Projection) any {
	if proj == nil || len(proj.fields) == 0 {
		return result
	}

	// Convert result to bson.M for manipulation
	bytes, err := bson.Marshal(result)
	if err != nil {
		return result
	}

	var doc bson.M
	if err := bson.Unmarshal(bytes, &doc); err != nil {
		return result
	}

	// Determine if it's inclusion or exclusion mode
	hasInclusion := false
	hasExclusion := false
	for _, value := range proj.fields {
		if value == 1 {
			hasInclusion = true
		} else if value == 0 {
			hasExclusion = true
		}
	}

	if hasInclusion {
		// Inclusion mode: only keep specified fields
		filtered := bson.M{}
		for field := range proj.fields {
			if val, exists := doc[field]; exists && proj.fields[field] == 1 {
				filtered[field] = val
			}
		}
		return filtered
	} else if hasExclusion {
		// Exclusion mode: remove specified fields
		for field := range proj.fields {
			if proj.fields[field] == 0 {
				delete(doc, field)
			}
		}
		return doc
	}

	return result
}

// applyProjectionToSlice applies projection to a slice of results
func applyProjectionToSlice(results any, proj *Projection) any {
	if proj == nil || len(proj.fields) == 0 {
		return results
	}

	// Try to convert to slice
	slice, ok := results.([]any)
	if !ok {
		return results
	}

	projected := make([]any, len(slice))
	for i, item := range slice {
		projected[i] = applyProjection(item, proj)
	}
	return projected
}

// UpdateOne implements DatabaseInterface
func (m *MockDatabase) UpdateOne(ctx context.Context, db string, collection string, filter any, update any, opts ...any) (UpdateResultInterface, error) {
	m.UpdateOneCalls = append(m.UpdateOneCalls, UpdateOneCall{
		Ctx:        ctx,
		Db:         db,
		Collection: collection,
		Filter:     filter,
		Update:     update,
		Opts:       opts,
	})

	// Check if there's a queued response
	if len(m.UpdateOneQueue) > 0 {
		response := m.UpdateOneQueue[0]
		m.UpdateOneQueue = m.UpdateOneQueue[1:]
		if response.Err != nil {
			return nil, response.Err
		}
		return &MockUpdateResult{
			matchedCount:  response.MatchedCount,
			modifiedCount: response.ModifiedCount,
			upsertedCount: response.UpsertedCount,
			upsertedID:    response.UpsertedID,
		}, nil
	}

	// Fall back to UpdateOneFunc
	if m.UpdateOneFunc != nil {
		return m.UpdateOneFunc(ctx, db, collection, filter, update, opts...)
	}
	return &MockUpdateResult{matchedCount: 1, modifiedCount: 1}, nil
}

// DeleteOne implements DatabaseInterface
func (m *MockDatabase) DeleteOne(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error) {
	m.DeleteOneCalls = append(m.DeleteOneCalls, DeleteCall{
		Ctx:        ctx,
		Db:         db,
		Collection: collection,
		Filter:     filter,
		Opts:       opts,
	})

	if len(m.DeleteOneQueue) > 0 {
		response := m.DeleteOneQueue[0]
		m.DeleteOneQueue = m.DeleteOneQueue[1:]
		if response.Err != nil {
			return nil, response.Err
		}
		return &MockDeleteResult{deletedCount: response.DeletedCount}, nil
	}

	if m.DeleteOneFunc != nil {
		return m.DeleteOneFunc(ctx, db, collection, filter, opts...)
	}

	return &MockDeleteResult{deletedCount: 1}, nil
}

// DeleteMany implements DatabaseInterface
func (m *MockDatabase) DeleteMany(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error) {
	m.DeleteManyCalls = append(m.DeleteManyCalls, DeleteCall{
		Ctx:        ctx,
		Db:         db,
		Collection: collection,
		Filter:     filter,
		Opts:       opts,
	})

	if len(m.DeleteManyQueue) > 0 {
		response := m.DeleteManyQueue[0]
		m.DeleteManyQueue = m.DeleteManyQueue[1:]
		if response.Err != nil {
			return nil, response.Err
		}
		return &MockDeleteResult{deletedCount: response.DeletedCount}, nil
	}

	if m.DeleteManyFunc != nil {
		return m.DeleteManyFunc(ctx, db, collection, filter, opts...)
	}

	return &MockDeleteResult{deletedCount: 0}, nil
}

// Count implements DatabaseInterface
func (m *MockDatabase) Count(ctx context.Context, db string, collection string, filter any, opts ...any) (int64, error) {
	m.CountCalls = append(m.CountCalls, CountCall{
		Ctx:        ctx,
		Db:         db,
		Collection: collection,
		Filter:     filter,
		Opts:       opts,
	})

	// Check if there's a queued response
	if len(m.CountQueue) > 0 {
		response := m.CountQueue[0]
		m.CountQueue = m.CountQueue[1:]
		return response.Count, response.Err
	}

	// Fall back to CountFunc
	if m.CountFunc != nil {
		return m.CountFunc(ctx, db, collection, filter, opts...)
	}
	return 0, nil
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
	m.FindOneAndUpdateCalls = []FindOneAndUpdateCall{}
	m.UpdateOneCalls = []UpdateOneCall{}
	m.DeleteOneCalls = []DeleteCall{}
	m.DeleteManyCalls = []DeleteCall{}
	m.CountCalls = []CountCall{}
	m.PingQueue = []PingResponse{}
	m.FindQueue = []FindResponse{}
	m.FindOneQueue = []FindOneResponse{}
	m.FindOneAndUpdateQueue = []FindOneResponse{}
	m.UpdateOneQueue = []UpdateOneResponse{}
	m.DeleteOneQueue = []DeleteResponse{}
	m.DeleteManyQueue = []DeleteResponse{}
	m.CountQueue = []CountResponse{}
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
	m.FindFunc = func(ctx context.Context, db string, collection string, filter any, opts ...any) FindResultInterface {
		return &MockFindResult{results: result, err: err}
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

// ExpectFindOneAndUpdate sets up an atomic find-and-update response.
func (m *MockDatabase) ExpectFindOneAndUpdate(result any, err error) *MockDatabase {
	m.FindOneAndUpdateFunc = func(ctx context.Context, db string, collection string, filter any, update any, opts ...any) SingleResultInterface {
		return &MockSingleResult{result: result, err: err}
	}
	return m
}

// ExpectUpdateOne sets up an expectation for UpdateOne
func (m *MockDatabase) ExpectUpdateOne(matchedCount, modifiedCount, upsertedCount int64, upsertedID any, err error) *MockDatabase {
	m.UpdateOneFunc = func(ctx context.Context, db string, collection string, filter any, update any, opts ...any) (UpdateResultInterface, error) {
		if err != nil {
			return nil, err
		}
		return &MockUpdateResult{
			matchedCount:  matchedCount,
			modifiedCount: modifiedCount,
			upsertedCount: upsertedCount,
			upsertedID:    upsertedID,
		}, nil
	}
	return m
}

// ExpectDeleteOne sets up an expectation for DeleteOne
func (m *MockDatabase) ExpectDeleteOne(deletedCount int64, err error) *MockDatabase {
	m.DeleteOneFunc = func(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error) {
		if err != nil {
			return nil, err
		}
		return &MockDeleteResult{deletedCount: deletedCount}, nil
	}
	return m
}

// ExpectDeleteMany sets up an expectation for DeleteMany
func (m *MockDatabase) ExpectDeleteMany(deletedCount int64, err error) *MockDatabase {
	m.DeleteManyFunc = func(ctx context.Context, db string, collection string, filter any, opts ...any) (DeleteResultInterface, error) {
		if err != nil {
			return nil, err
		}
		return &MockDeleteResult{deletedCount: deletedCount}, nil
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

// QueueFindOneAndUpdate adds an atomic find-and-update response to the queue.
func (m *MockDatabase) QueueFindOneAndUpdate(result any, err error) *MockDatabase {
	m.FindOneAndUpdateQueue = append(m.FindOneAndUpdateQueue, FindOneResponse{Result: result, Err: err})
	return m
}

// QueueUpdateOne adds an UpdateOne response to the queue for sequential calls
func (m *MockDatabase) QueueUpdateOne(matchedCount, modifiedCount, upsertedCount int64, upsertedID any, err error) *MockDatabase {
	m.UpdateOneQueue = append(m.UpdateOneQueue, UpdateOneResponse{
		MatchedCount:  matchedCount,
		ModifiedCount: modifiedCount,
		UpsertedCount: upsertedCount,
		UpsertedID:    upsertedID,
		Err:           err,
	})
	return m
}

// QueueDeleteOne adds a DeleteOne response to the queue for sequential calls
func (m *MockDatabase) QueueDeleteOne(deletedCount int64, err error) *MockDatabase {
	m.DeleteOneQueue = append(m.DeleteOneQueue, DeleteResponse{DeletedCount: deletedCount, Err: err})
	return m
}

// QueueDeleteMany adds a DeleteMany response to the queue for sequential calls
func (m *MockDatabase) QueueDeleteMany(deletedCount int64, err error) *MockDatabase {
	m.DeleteManyQueue = append(m.DeleteManyQueue, DeleteResponse{DeletedCount: deletedCount, Err: err})
	return m
}

// ExpectCount sets up an expectation for Count
func (m *MockDatabase) ExpectCount(count int64, err error) *MockDatabase {
	m.CountFunc = func(ctx context.Context, db string, collection string, filter any, opts ...any) (int64, error) {
		return count, err
	}
	return m
}

// QueueCount adds a Count response to the queue for sequential calls
func (m *MockDatabase) QueueCount(count int64, err error) *MockDatabase {
	m.CountQueue = append(m.CountQueue, CountResponse{Count: count, Err: err})
	return m
}
