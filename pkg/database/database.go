package database

type DatabaseInterface interface {
	Ping() error
}
