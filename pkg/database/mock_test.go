package database

import (
	"context"
	"errors"
	"fmt"
	"testing"
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
		result, err := mock.Find(context.Background(), "testdb", "users", map[string]any{"id": 1})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}

		// Test FindOne default (should return error)
		result, err = mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 1})
		if err == nil {
			t.Error("expected error, got nil")
		}
		if result != nil {
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

		result, err := mock.Find(context.Background(), "testdb", "users", map[string]any{})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}

		resultSlice, ok := result.([]map[string]any)
		if !ok {
			t.Fatal("expected result to be []map[string]any")
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

	t.Run("ExpectFindOneWithResult", func(t *testing.T) {
		mock := NewMockDatabase()
		expectedUser := map[string]any{
			"id":    1,
			"name":  "Alice",
			"email": "alice@example.com",
		}

		mock.ExpectFindOne(expectedUser, nil)

		result, err := mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 1})
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

		result, err := mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 999})
		if err != expectedErr {
			t.Errorf("expected error '%v', got '%v'", expectedErr, err)
		}
		if result != nil {
			t.Error("expected nil result")
		}
	})

	t.Run("CustomFindFunction", func(t *testing.T) {
		mock := NewMockDatabase()

		// Custom function that returns different results based on filter
		mock.FindFunc = func(ctx context.Context, db string, collection string, filter any, opts ...any) (any, error) {
			filterMap, ok := filter.(map[string]any)
			if !ok {
				return nil, errors.New("invalid filter")
			}

			if status, ok := filterMap["status"]; ok && status == "active" {
				return []map[string]any{
					{"id": 1, "status": "active"},
					{"id": 2, "status": "active"},
				}, nil
			}

			return []map[string]any{}, nil
		}

		// Test with active status
		result, err := mock.Find(context.Background(), "testdb", "users", map[string]any{"status": "active"})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if len(result.([]map[string]any)) != 2 {
			t.Errorf("expected 2 results for active users")
		}

		// Test with inactive status
		result, err = mock.Find(context.Background(), "testdb", "users", map[string]any{"status": "inactive"})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if len(result.([]map[string]any)) != 0 {
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
		result1, err := mock.Find(context.Background(), "testdb", "users", map[string]any{})
		if err != nil {
			t.Errorf("unexpected error on first call: %v", err)
		}
		usersResult := result1.([]map[string]any)
		if len(usersResult) != 2 || usersResult[0]["name"] != "Alice" {
			t.Error("first call should return users")
		}

		// Second call returns notifications
		result2, err := mock.Find(context.Background(), "testdb", "notifications", map[string]any{})
		if err != nil {
			t.Errorf("unexpected error on second call: %v", err)
		}
		notificationsResult := result2.([]map[string]any)
		if len(notificationsResult) != 2 || notificationsResult[0]["message"] != "Hello" {
			t.Error("second call should return notifications")
		}

		// Third call returns settings
		result3, err := mock.Find(context.Background(), "testdb", "settings", map[string]any{})
		if err != nil {
			t.Errorf("unexpected error on third call: %v", err)
		}
		settingsResult := result3.([]map[string]any)
		if len(settingsResult) != 1 || settingsResult[0]["key"] != "theme" {
			t.Error("third call should return settings")
		}

		// Fourth call falls back to default behavior (empty slice)
		result4, err := mock.Find(context.Background(), "testdb", "other", map[string]any{})
		if err != nil {
			t.Errorf("unexpected error on fourth call: %v", err)
		}
		if len(result4.([]any)) != 0 {
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
		result1, err := mock.Find(context.Background(), "testdb", "users", map[string]any{})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(result1.([]map[string]any)) != 1 {
			t.Error("first call should return 1 result")
		}

		// Second call returns error
		_, err = mock.Find(context.Background(), "testdb", "users", map[string]any{})
		if err == nil || err.Error() != "connection timeout" {
			t.Errorf("expected 'connection timeout' error, got %v", err)
		}

		// Third call succeeds again
		result3, err := mock.Find(context.Background(), "testdb", "users", map[string]any{})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(result3.([]map[string]any)) != 1 {
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
		result1, err := mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 1})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result1.(map[string]any)["name"] != "Alice" {
			t.Error("first call should return Alice")
		}

		// Second call
		result2, err := mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 2})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result2.(map[string]any)["name"] != "Bob" {
			t.Error("second call should return Bob")
		}

		// Third call returns error
		_, err = mock.FindOne(context.Background(), "testdb", "users", map[string]any{"id": 3})
		if err == nil || err.Error() != "not found" {
			t.Errorf("expected 'not found' error, got %v", err)
		}
	})

	t.Run("ResetClearsQueue", func(t *testing.T) {
		mock := NewMockDatabase()

		// Queue responses
		mock.QueueFind([]map[string]any{{"id": 1}}, nil).
			QueueFindOne(map[string]any{"id": 1}, nil)

		// Reset should clear queues
		mock.Reset()

		if len(mock.FindQueue) != 0 {
			t.Error("FindQueue should be empty after Reset")
		}
		if len(mock.FindOneQueue) != 0 {
			t.Error("FindOneQueue should be empty after Reset")
		}
	})
}
