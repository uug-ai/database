package database

// Option is a generic functional option pattern
type Option[T any] func(*T)
