package database

// sqlc reads the migrations and the files in queries/, and writes typed Go functions to internal/store.
//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate -f ../../sqlc.yaml
