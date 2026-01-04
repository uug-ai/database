package database

import (
	"context"
	"errors"
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
