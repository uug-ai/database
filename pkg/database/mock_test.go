package database

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestMockDatabase(t *testing.T) {
	t.Run("DefaultBehavior", func(t *testing.T) {
		mock := NewMockDatabase()

		// Test Ping default (should succeed)
		err := mock.Ping(context.Background())
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}

		// Test Find default (should return empty slice)
		var results []any
		err = mock.Find(context.Background(), "testdb", "users", map[string]any{"id": 1}).All(&results)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if results == nil {
			t.Error("expected non-nil result")
		}

		// Test FindOne default (should return error)
		result2, err := mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 1}).Raw()
		if err == nil {
			t.Error("expected error, got nil")
		}
		if result2 != nil {
			t.Error("expected nil result")
		}
	})

	t.Run("ExpectPingError", func(t *testing.T) {
		mock := NewMockDatabase()
		expectedErr := errors.New("connection failed")

		mock.ExpectPing(expectedErr)

		err := mock.Ping(context.Background())
		if err != expectedErr {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}

		// Verify call was tracked
		if len(mock.PingCalls) != 1 {
			t.Errorf("expected 1 ping call, got %d", len(mock.PingCalls))
		}
	})

	t.Run("ExpectFindWithResults", func(t *testing.T) {
		mock := NewMockDatabase()
		expectedData := []map[string]any{
			{"id": 1, "name": "Alice"},
			{"id": 2, "name": "Bob"},
		}

		mock.ExpectFind(expectedData, nil)

		var resultSlice []map[string]any
		err := mock.Find(context.Background(), "testdb", "users", map[string]any{}).All(&resultSlice)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}

		if len(resultSlice) != 2 {
			t.Errorf("expected 2 results, got %d", len(resultSlice))
		}

		// Verify call tracking
		if len(mock.FindCalls) != 1 {
			t.Errorf("expected 1 find call, got %d", len(mock.FindCalls))
		}
		if mock.FindCalls[0].Db != "testdb" {
			t.Errorf("expected db 'testdb', got '%s'", mock.FindCalls[0].Db)
		}
		if mock.FindCalls[0].Collection != "users" {
			t.Errorf("expected collection 'users', got '%s'", mock.FindCalls[0].Collection)
		}
	})

	t.Run("CursorIterationSkipsIndividualDecodeError", func(t *testing.T) {
		mock := NewMockDatabase().QueueFind([]bson.M{
			{"name": "first"},
			{"name": []string{"invalid"}},
			{"name": "third"},
		}, nil)

		result := mock.Find(context.Background(), "testdb", "users", bson.M{})
		cursor, ok := result.(CursorResultInterface)
		if !ok {
			t.Fatal("mock find result must support cursor iteration")
		}

		var names []string
		decodeErrors := 0
		for cursor.Next(context.Background()) {
			var document struct {
				Name string `bson:"name"`
			}
			if err := cursor.Decode(&document); err != nil {
				decodeErrors++
				continue
			}
			names = append(names, document.Name)
		}
		if err := cursor.Err(); err != nil {
			t.Fatalf("cursor error: %v", err)
		}
		if err := cursor.Close(context.Background()); err != nil {
			t.Fatalf("close cursor: %v", err)
		}
		if decodeErrors != 1 || len(names) != 2 || names[0] != "first" || names[1] != "third" {
			t.Fatalf("decode errors = %d, names = %v", decodeErrors, names)
		}
	})

	t.Run("CursorIterationPreservesFindError", func(t *testing.T) {
		expectedErr := errors.New("find failed")
		result := NewMockDatabase().QueueFind(nil, expectedErr).
			Find(context.Background(), "testdb", "users", bson.M{})
		cursor := result.(CursorResultInterface)

		if cursor.Next(context.Background()) {
			t.Fatal("failed find must not have a next document")
		}
		if !errors.Is(cursor.Err(), expectedErr) {
			t.Fatalf("cursor error = %v, want %v", cursor.Err(), expectedErr)
		}
		if !errors.Is(cursor.Decode(&bson.M{}), expectedErr) {
			t.Fatalf("decode error = %v, want %v", cursor.Decode(&bson.M{}), expectedErr)
		}
		if !errors.Is(cursor.Close(context.Background()), expectedErr) {
			t.Fatalf("close error = %v, want %v", cursor.Close(context.Background()), expectedErr)
		}
	})

	t.Run("ExpectFindOneWithResult", func(t *testing.T) {
		mock := NewMockDatabase()
		expectedUser := map[string]any{
			"id":    1,
			"name":  "Alice",
			"email": "alice@example.com",
		}

		mock.ExpectFindOne(expectedUser, nil)

		result, err := mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 1}).Raw()
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}

		user, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected result to be map[string]any")
		}

		if user["name"] != "Alice" {
			t.Errorf("expected name 'Alice', got '%v'", user["name"])
		}

		// Verify call tracking
		if len(mock.FindOneCalls) != 1 {
			t.Errorf("expected 1 findOne call, got %d", len(mock.FindOneCalls))
		}
	})

	t.Run("ExpectFindOneNotFound", func(t *testing.T) {
		mock := NewMockDatabase()
		expectedErr := errors.New("document not found")

		mock.ExpectFindOne(nil, expectedErr)

		result, err := mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 999}).Raw()
		if err != expectedErr {
			t.Errorf("expected error '%v', got '%v'", expectedErr, err)
		}
		if result != nil {
			t.Error("expected nil result")
		}
	})

	t.Run("ExpectFindOneAndUpdateWithResult", func(t *testing.T) {
		mock := NewMockDatabase()
		expected := map[string]any{"id": 1, "name": "Updated"}
		filter := map[string]any{"id": 1}
		update := map[string]any{"$set": map[string]any{"name": "Updated"}}
		mock.ExpectFindOneAndUpdate(expected, nil)

		var result map[string]any
		err := mock.FindOneAndUpdate(context.Background(), "testdb", "users", filter, update).Into(&result)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if result["name"] != "Updated" {
			t.Fatalf("expected updated document, got %#v", result)
		}
		if len(mock.FindOneAndUpdateCalls) != 1 {
			t.Fatalf("expected 1 findOneAndUpdate call, got %d", len(mock.FindOneAndUpdateCalls))
		}
		call := mock.FindOneAndUpdateCalls[0]
		if call.Db != "testdb" || call.Collection != "users" {
			t.Fatalf("unexpected call target %s.%s", call.Db, call.Collection)
		}
	})

	t.Run("QueueFindOneAndUpdateErrorAndReset", func(t *testing.T) {
		mock := NewMockDatabase().QueueFindOneAndUpdate(nil, errors.New("update failed"))

		err := mock.FindOneAndUpdate(context.Background(), "testdb", "users", map[string]any{"id": 1}, map[string]any{"$set": map[string]any{"name": "Updated"}}).Err()
		if err == nil || err.Error() != "update failed" {
			t.Fatalf("expected update failure, got %v", err)
		}
		if len(mock.FindOneAndUpdateCalls) != 1 {
			t.Fatalf("expected 1 findOneAndUpdate call, got %d", len(mock.FindOneAndUpdateCalls))
		}

		mock.Reset()
		if len(mock.FindOneAndUpdateCalls) != 0 || len(mock.FindOneAndUpdateQueue) != 0 {
			t.Fatal("expected findOneAndUpdate state to be cleared")
		}
	})

	t.Run("ExpectCreateIndexesWithCallTracking", func(t *testing.T) {
		mock := NewMockDatabase().ExpectCreateIndexes([]string{"ownerId_1"}, nil)
		indexes := []mongo.IndexModel{{Keys: bson.D{{Key: "ownerId", Value: 1}}}}
		opts := options.CreateIndexes().SetMaxTime(10)

		names, err := mock.CreateIndexes(context.Background(), "testdb", "organisation", indexes, opts)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if len(names) != 1 || names[0] != "ownerId_1" {
			t.Fatalf("created index names = %v", names)
		}
		if len(mock.CreateIndexesCalls) != 1 {
			t.Fatalf("expected 1 CreateIndexes call, got %d", len(mock.CreateIndexesCalls))
		}
		call := mock.CreateIndexesCalls[0]
		if call.Db != "testdb" || call.Collection != "organisation" || len(call.Indexes) != 1 {
			t.Fatalf("unexpected CreateIndexes call: %#v", call)
		}
		if len(call.Opts) != 1 || call.Opts[0] != opts {
			t.Fatalf("CreateIndexes options = %#v", call.Opts)
		}
	})

	t.Run("QueueCreateIndexesErrorAndReset", func(t *testing.T) {
		expectedErr := errors.New("index creation failed")
		mock := NewMockDatabase().QueueCreateIndexes(nil, expectedErr)

		_, err := mock.CreateIndexes(context.Background(), "testdb", "organisation", nil)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected index creation failure, got %v", err)
		}
		if len(mock.CreateIndexesCalls) != 1 {
			t.Fatalf("expected 1 CreateIndexes call, got %d", len(mock.CreateIndexesCalls))
		}

		mock.Reset()
		if len(mock.CreateIndexesCalls) != 0 || len(mock.CreateIndexesQueue) != 0 {
			t.Fatal("expected CreateIndexes state to be cleared")
		}
	})

	t.Run("CustomFindFunction", func(t *testing.T) {
		mock := NewMockDatabase()

		// Custom function that returns different results based on filter
		mock.FindFunc = func(ctx context.Context, db string, collection string, filter any, opts ...any) FindResultInterface {
			filterMap, ok := filter.(map[string]any)
			if !ok {
				return &MockFindResult{results: nil, err: errors.New("invalid filter")}
			}

			if status, ok := filterMap["status"]; ok && status == "active" {
				return &MockFindResult{results: []map[string]any{
					{"id": 1, "status": "active"},
					{"id": 2, "status": "active"},
				}, err: nil}
			}

			return &MockFindResult{results: []map[string]any{}, err: nil}
		}

		// Test with active status
		var activeResults []map[string]any
		err := mock.Find(context.Background(), "testdb", "users", map[string]any{"status": "active"}).All(&activeResults)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if len(activeResults) != 2 {
			t.Errorf("expected 2 results for active users")
		}

		// Test with inactive status
		var inactiveResults []map[string]any
		err = mock.Find(context.Background(), "testdb", "users", map[string]any{"status": "inactive"}).All(&inactiveResults)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if len(inactiveResults) != 0 {
			t.Errorf("expected 0 results for inactive users")
		}

		// Verify both calls were tracked
		if len(mock.FindCalls) != 2 {
			t.Errorf("expected 2 find calls, got %d", len(mock.FindCalls))
		}
	})

	t.Run("ResetCallHistory", func(t *testing.T) {
		mock := NewMockDatabase()

		// Make some calls
		mock.Ping(context.Background())
		mock.Find(context.Background(), "testdb", "users", map[string]any{})
		mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 1})

		if len(mock.PingCalls) != 1 || len(mock.FindCalls) != 1 || len(mock.FindOneCalls) != 1 {
			t.Error("expected calls to be tracked")
		}

		// Reset
		mock.Reset()

		if len(mock.PingCalls) != 0 || len(mock.FindCalls) != 0 || len(mock.FindOneCalls) != 0 {
			t.Error("expected all call history to be cleared")
		}
	})

	t.Run("UseWithDatabase", func(t *testing.T) {
		mock := NewMockDatabase()
		mock.ExpectPing(nil)

		opts := NewMongoOptions().
			SetUri("mongodb://localhost").
			SetTimeout(5000).
			Build()

		// Inject the mock as the database client
		db, err := New(opts, mock)
		if err != nil {
			t.Fatalf("failed to create database with mock: %v", err)
		}

		// Use the database with the mock
		err = db.Client.Ping(context.Background())
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}

		// Verify the mock was called
		if len(mock.PingCalls) != 1 {
			t.Errorf("expected 1 ping call on mock, got %d", len(mock.PingCalls))
		}
	})

	t.Run("ExpectDeleteOperations", func(t *testing.T) {
		mock := NewMockDatabase()
		mock.ExpectDeleteOne(1, nil)
		mock.ExpectDeleteMany(3, nil)

		deleteOneResult, err := mock.DeleteOne(context.Background(), "testdb", "users", map[string]any{"id": 1})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if deleteOneResult.DeletedCount() != 1 {
			t.Errorf("expected 1 deleted document, got %d", deleteOneResult.DeletedCount())
		}

		deleteManyResult, err := mock.DeleteMany(context.Background(), "testdb", "users", map[string]any{"inactive": true})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if deleteManyResult.DeletedCount() != 3 {
			t.Errorf("expected 3 deleted documents, got %d", deleteManyResult.DeletedCount())
		}

		if len(mock.DeleteOneCalls) != 1 {
			t.Errorf("expected 1 deleteOne call, got %d", len(mock.DeleteOneCalls))
		}
		if len(mock.DeleteManyCalls) != 1 {
			t.Errorf("expected 1 deleteMany call, got %d", len(mock.DeleteManyCalls))
		}
	})
}

func TestMockDatabaseSequentialCalls(t *testing.T) {
	t.Run("QueueMultipleFinds", func(t *testing.T) {
		mock := NewMockDatabase()

		// Queue multiple responses
		users := []map[string]any{
			{"id": 1, "name": "Alice"},
			{"id": 2, "name": "Bob"},
		}
		notifications := []map[string]any{
			{"id": 1, "message": "Hello"},
			{"id": 2, "message": "World"},
		}
		settings := []map[string]any{
			{"key": "theme", "value": "dark"},
		}

		mock.QueueFind(users, nil).
			QueueFind(notifications, nil).
			QueueFind(settings, nil)

		// First call returns users
		var usersResult []map[string]any
		err := mock.Find(context.Background(), "testdb", "users", map[string]any{}).All(&usersResult)
		if err != nil {
			t.Errorf("unexpected error on first call: %v", err)
		}
		if len(usersResult) != 2 || usersResult[0]["name"] != "Alice" {
			t.Error("first call should return users")
		}

		// Second call returns notifications
		var notificationsResult []map[string]any
		err = mock.Find(context.Background(), "testdb", "notifications", map[string]any{}).All(&notificationsResult)
		if err != nil {
			t.Errorf("unexpected error on second call: %v", err)
		}
		if len(notificationsResult) != 2 || notificationsResult[0]["message"] != "Hello" {
			t.Error("second call should return notifications")
		}

		// Third call returns settings
		var settingsResult []map[string]any
		err = mock.Find(context.Background(), "testdb", "settings", map[string]any{}).All(&settingsResult)
		if err != nil {
			t.Errorf("unexpected error on third call: %v", err)
		}
		if len(settingsResult) != 1 || settingsResult[0]["key"] != "theme" {
			t.Error("third call should return settings")
		}

		// Fourth call falls back to default behavior (empty slice)
		var otherResult []any
		err = mock.Find(context.Background(), "testdb", "other", map[string]any{}).All(&otherResult)
		if err != nil {
			t.Errorf("unexpected error on fourth call: %v", err)
		}
		if len(otherResult) != 0 {
			t.Error("fourth call should return empty slice (default)")
		}

		// Verify all calls were tracked
		if len(mock.FindCalls) != 4 {
			t.Errorf("expected 4 find calls, got %d", len(mock.FindCalls))
		}
	})

	t.Run("QueueWithErrors", func(t *testing.T) {
		mock := NewMockDatabase()

		// Queue responses with errors
		mock.QueueFind([]map[string]any{{"id": 1}}, nil).
			QueueFind(nil, fmt.Errorf("connection timeout")).
			QueueFind([]map[string]any{{"id": 2}}, nil)

		// First call succeeds
		var result1 []map[string]any
		err := mock.Find(context.Background(), "testdb", "users", map[string]any{}).All(&result1)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(result1) != 1 {
			t.Error("first call should return 1 result")
		}

		// Second call returns error
		var result2 []map[string]any
		err = mock.Find(context.Background(), "testdb", "users", map[string]any{}).All(&result2)
		if err == nil || err.Error() != "connection timeout" {
			t.Errorf("expected 'connection timeout' error, got %v", err)
		}

		// Third call succeeds again
		var result3 []map[string]any
		err = mock.Find(context.Background(), "testdb", "users", map[string]any{}).All(&result3)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(result3) != 1 {
			t.Error("third call should return 1 result")
		}
	})

	t.Run("QueueFindOne", func(t *testing.T) {
		mock := NewMockDatabase()

		// Queue multiple FindOne responses
		mock.QueueFindOne(map[string]any{"id": 1, "name": "Alice"}, nil).
			QueueFindOne(map[string]any{"id": 2, "name": "Bob"}, nil).
			QueueFindOne(nil, fmt.Errorf("not found"))

		// First call
		result1, err := mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 1}).Raw()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result1.(map[string]any)["name"] != "Alice" {
			t.Error("first call should return Alice")
		}

		// Second call
		result2, err := mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 2}).Raw()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result2.(map[string]any)["name"] != "Bob" {
			t.Error("second call should return Bob")
		}

		// Third call returns error
		_, err = mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 3}).Raw()
		if err == nil || err.Error() != "not found" {
			t.Errorf("expected 'not found' error, got %v", err)
		}
	})

	t.Run("ResetClearsQueue", func(t *testing.T) {
		mock := NewMockDatabase()

		// Queue responses
		mock.QueueFind([]map[string]any{{"id": 1}}, nil).
			QueueFindOne(map[string]any{"id": 1}, nil).
			QueueDeleteOne(1, nil).
			QueueDeleteMany(2, nil)

		// Reset should clear queues
		mock.Reset()

		if len(mock.FindQueue) != 0 {
			t.Error("FindQueue should be empty after Reset")
		}
		if len(mock.FindOneQueue) != 0 {
			t.Error("FindOneQueue should be empty after Reset")
		}
		if len(mock.DeleteOneQueue) != 0 {
			t.Error("DeleteOneQueue should be empty after Reset")
		}
		if len(mock.DeleteManyQueue) != 0 {
			t.Error("DeleteManyQueue should be empty after Reset")
		}
		if len(mock.CountQueue) != 0 {
			t.Error("CountQueue should be empty after Reset")
		}
	})
}

func TestMockDatabaseDelete(t *testing.T) {
	t.Run("DefaultBehavior", func(t *testing.T) {
		mock := NewMockDatabase()

		deleteOneResult, err := mock.DeleteOne(context.Background(), "testdb", "users", map[string]any{"id": 1})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if deleteOneResult.DeletedCount() != 1 {
			t.Errorf("expected deleted count 1, got %d", deleteOneResult.DeletedCount())
		}

		deleteManyResult, err := mock.DeleteMany(context.Background(), "testdb", "users", map[string]any{"inactive": true})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if deleteManyResult.DeletedCount() != 0 {
			t.Errorf("expected deleted count 0, got %d", deleteManyResult.DeletedCount())
		}
	})

	t.Run("ExpectDeleteOneWithError", func(t *testing.T) {
		mock := NewMockDatabase()
		expectedErr := fmt.Errorf("delete failed")
		mock.ExpectDeleteOne(0, expectedErr)

		result, err := mock.DeleteOne(context.Background(), "testdb", "users", map[string]any{"id": 1})
		if err != expectedErr {
			t.Errorf("expected error '%v', got '%v'", expectedErr, err)
		}
		if result != nil {
			t.Error("expected nil result on error")
		}
	})

	t.Run("QueueMultipleDeletes", func(t *testing.T) {
		mock := NewMockDatabase()

		mock.QueueDeleteOne(1, nil).
			QueueDeleteOne(0, fmt.Errorf("timeout")).
			QueueDeleteMany(4, nil).
			QueueDeleteMany(0, fmt.Errorf("denied"))

		result, err := mock.DeleteOne(context.Background(), "testdb", "users", map[string]any{"id": 1})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if result.DeletedCount() != 1 {
			t.Errorf("expected deleted count 1, got %d", result.DeletedCount())
		}

		result, err = mock.DeleteOne(context.Background(), "testdb", "users", map[string]any{"id": 2})
		if err == nil || err.Error() != "timeout" {
			t.Errorf("expected 'timeout' error, got %v", err)
		}
		if result != nil {
			t.Error("expected nil result on queued error")
		}

		result, err = mock.DeleteMany(context.Background(), "testdb", "users", map[string]any{"inactive": true})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if result.DeletedCount() != 4 {
			t.Errorf("expected deleted count 4, got %d", result.DeletedCount())
		}

		result, err = mock.DeleteMany(context.Background(), "testdb", "users", map[string]any{"expired": true})
		if err == nil || err.Error() != "denied" {
			t.Errorf("expected 'denied' error, got %v", err)
		}
		if result != nil {
			t.Error("expected nil result on queued error")
		}

		if len(mock.DeleteOneCalls) != 2 {
			t.Errorf("expected 2 deleteOne calls, got %d", len(mock.DeleteOneCalls))
		}
		if len(mock.DeleteManyCalls) != 2 {
			t.Errorf("expected 2 deleteMany calls, got %d", len(mock.DeleteManyCalls))
		}
	})

	t.Run("ResetClearsDeleteState", func(t *testing.T) {
		mock := NewMockDatabase()

		mock.QueueDeleteOne(1, nil).
			QueueDeleteMany(2, nil)

		mock.DeleteOne(context.Background(), "testdb", "users", map[string]any{"id": 1})
		mock.DeleteMany(context.Background(), "testdb", "users", map[string]any{"inactive": true})

		if len(mock.DeleteOneCalls) != 1 || len(mock.DeleteManyCalls) != 1 {
			t.Error("expected delete calls to be tracked")
		}

		mock.Reset()

		if len(mock.DeleteOneCalls) != 0 {
			t.Error("expected deleteOne calls to be cleared after reset")
		}
		if len(mock.DeleteManyCalls) != 0 {
			t.Error("expected deleteMany calls to be cleared after reset")
		}
		if len(mock.DeleteOneQueue) != 0 {
			t.Error("expected deleteOne queue to be cleared after reset")
		}
		if len(mock.DeleteManyQueue) != 0 {
			t.Error("expected deleteMany queue to be cleared after reset")
		}
	})
}

func TestMockDatabaseCount(t *testing.T) {
	t.Run("DefaultBehavior", func(t *testing.T) {
		mock := NewMockDatabase()

		count, err := mock.Count(context.Background(), "testdb", "users", map[string]any{})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if count != 0 {
			t.Errorf("expected count 0, got %d", count)
		}

		// Verify call tracking
		if len(mock.CountCalls) != 1 {
			t.Errorf("expected 1 count call, got %d", len(mock.CountCalls))
		}
		if mock.CountCalls[0].Db != "testdb" {
			t.Errorf("expected db 'testdb', got '%s'", mock.CountCalls[0].Db)
		}
		if mock.CountCalls[0].Collection != "users" {
			t.Errorf("expected collection 'users', got '%s'", mock.CountCalls[0].Collection)
		}
	})

	t.Run("ExpectCount", func(t *testing.T) {
		mock := NewMockDatabase()
		mock.ExpectCount(42, nil)

		count, err := mock.Count(context.Background(), "testdb", "users", map[string]any{"status": "active"})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if count != 42 {
			t.Errorf("expected count 42, got %d", count)
		}
	})

	t.Run("ExpectCountWithError", func(t *testing.T) {
		mock := NewMockDatabase()
		expectedErr := fmt.Errorf("connection failed")
		mock.ExpectCount(0, expectedErr)

		count, err := mock.Count(context.Background(), "testdb", "users", map[string]any{})
		if err != expectedErr {
			t.Errorf("expected error '%v', got '%v'", expectedErr, err)
		}
		if count != 0 {
			t.Errorf("expected count 0, got %d", count)
		}
	})

	t.Run("QueueMultipleCounts", func(t *testing.T) {
		mock := NewMockDatabase()

		mock.QueueCount(10, nil).
			QueueCount(0, fmt.Errorf("timeout")).
			QueueCount(25, nil)

		// First call
		count, err := mock.Count(context.Background(), "testdb", "users", map[string]any{})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if count != 10 {
			t.Errorf("expected count 10, got %d", count)
		}

		// Second call returns error
		count, err = mock.Count(context.Background(), "testdb", "users", map[string]any{})
		if err == nil || err.Error() != "timeout" {
			t.Errorf("expected 'timeout' error, got %v", err)
		}
		if count != 0 {
			t.Errorf("expected count 0, got %d", count)
		}

		// Third call succeeds
		count, err = mock.Count(context.Background(), "testdb", "orders", map[string]any{})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if count != 25 {
			t.Errorf("expected count 25, got %d", count)
		}

		// Fourth call falls back to default
		count, err = mock.Count(context.Background(), "testdb", "other", map[string]any{})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if count != 0 {
			t.Errorf("expected count 0 (default), got %d", count)
		}

		// Verify all calls tracked
		if len(mock.CountCalls) != 4 {
			t.Errorf("expected 4 count calls, got %d", len(mock.CountCalls))
		}
	})

	t.Run("CustomCountFunc", func(t *testing.T) {
		mock := NewMockDatabase()

		mock.CountFunc = func(ctx context.Context, db string, collection string, filter any, opts ...any) (int64, error) {
			if collection == "users" {
				return 100, nil
			}
			return 0, fmt.Errorf("unknown collection")
		}

		count, err := mock.Count(context.Background(), "testdb", "users", map[string]any{})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if count != 100 {
			t.Errorf("expected count 100, got %d", count)
		}

		count, err = mock.Count(context.Background(), "testdb", "unknown", map[string]any{})
		if err == nil {
			t.Error("expected error for unknown collection")
		}
		if count != 0 {
			t.Errorf("expected count 0, got %d", count)
		}

		if len(mock.CountCalls) != 2 {
			t.Errorf("expected 2 count calls, got %d", len(mock.CountCalls))
		}
	})

	t.Run("ResetClearsCountState", func(t *testing.T) {
		mock := NewMockDatabase()

		mock.QueueCount(5, nil)
		mock.Count(context.Background(), "testdb", "users", map[string]any{})

		if len(mock.CountCalls) != 1 {
			t.Error("expected count call to be tracked")
		}

		mock.Reset()

		if len(mock.CountCalls) != 0 {
			t.Error("expected count calls to be cleared after reset")
		}
		if len(mock.CountQueue) != 0 {
			t.Error("expected count queue to be cleared after reset")
		}
	})
}

func TestMockDatabaseInsert(t *testing.T) {
	t.Run("ExpectInsertOneTracksDocumentAndOptions", func(t *testing.T) {
		mock := NewMockDatabase().ExpectInsertOne("inserted-id", nil)
		document := bson.M{"name": "organisation"}
		opts := options.InsertOne().SetBypassDocumentValidation(true)

		result, err := mock.InsertOne(context.Background(), "testdb", "organisation", document, opts)
		if err != nil {
			t.Fatalf("InsertOne error: %v", err)
		}
		if result != "inserted-id" {
			t.Fatalf("InsertOne result = %#v", result)
		}
		if len(mock.InsertOneCalls) != 1 {
			t.Fatalf("InsertOne calls = %d", len(mock.InsertOneCalls))
		}
		call := mock.InsertOneCalls[0]
		if call.Db != "testdb" || call.Collection != "organisation" || len(call.Opts) != 1 || call.Opts[0] != opts {
			t.Fatalf("unexpected InsertOne call: %#v", call)
		}
	})

	t.Run("QueueInsertManyPreservesResponsesAndReset", func(t *testing.T) {
		expectedErr := errors.New("insert many failed")
		mock := NewMockDatabase().
			QueueInsertMany([]any{"first", "second"}, nil).
			QueueInsertMany(nil, expectedErr)
		documents := []any{bson.M{"name": "first"}, bson.M{"name": "second"}}
		opts := options.InsertMany().SetOrdered(false)

		result, err := mock.InsertMany(context.Background(), "testdb", "organisation", documents, opts)
		if err != nil {
			t.Fatalf("first InsertMany error: %v", err)
		}
		insertedIDs, ok := result.([]any)
		if !ok || len(insertedIDs) != 2 {
			t.Fatalf("first InsertMany result = %#v", result)
		}

		result, err = mock.InsertMany(context.Background(), "testdb", "organisation", documents)
		if !errors.Is(err, expectedErr) || result != nil {
			t.Fatalf("second InsertMany result = %#v, error = %v", result, err)
		}
		if len(mock.InsertManyCalls) != 2 || len(mock.InsertManyCalls[0].Opts) != 1 || mock.InsertManyCalls[0].Opts[0] != opts {
			t.Fatalf("InsertMany calls = %#v", mock.InsertManyCalls)
		}

		mock.Reset()
		if len(mock.InsertManyCalls) != 0 || len(mock.InsertManyQueue) != 0 {
			t.Fatal("expected InsertMany state to be cleared")
		}
	})

	t.Run("ExpectInsertManyReturnsConfiguredResult", func(t *testing.T) {
		mock := NewMockDatabase().ExpectInsertMany([]any{"first", "second"}, nil)

		result, err := mock.InsertMany(context.Background(), "testdb", "organisation", []any{bson.M{}, bson.M{}})
		if err != nil {
			t.Fatalf("InsertMany error: %v", err)
		}
		insertedIDs, ok := result.([]any)
		if !ok || len(insertedIDs) != 2 || insertedIDs[0] != "first" || insertedIDs[1] != "second" {
			t.Fatalf("InsertMany result = %#v", result)
		}
	})

	t.Run("QueueInsertOnePreservesErrorAndReset", func(t *testing.T) {
		expectedErr := errors.New("insert one failed")
		mock := NewMockDatabase().QueueInsertOne(nil, expectedErr)

		result, err := mock.InsertOne(context.Background(), "testdb", "organisation", bson.M{})
		if !errors.Is(err, expectedErr) || result != nil {
			t.Fatalf("InsertOne result = %#v, error = %v", result, err)
		}
		if len(mock.InsertOneCalls) != 1 {
			t.Fatalf("InsertOne calls = %d", len(mock.InsertOneCalls))
		}

		mock.Reset()
		if len(mock.InsertOneCalls) != 0 || len(mock.InsertOneQueue) != 0 {
			t.Fatal("expected InsertOne state to be cleared")
		}
	})
}
