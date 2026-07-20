package database_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
)

func TestOpenConnectionsConfiguresIsolatedReadOnlyPools(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	connections, err := database.OpenConnections(ctx, filepath.Join(t.TempDir(), "pools.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connections.Close() })

	if got := connections.Writer.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("writer max connections = %d, want 1", got)
	}
	if got := connections.ViewerReader.Stats().MaxOpenConnections; got != 4 {
		t.Fatalf("viewer reader max connections = %d, want 4", got)
	}
	if got := connections.AdminReader.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("admin reader max connections = %d, want 1", got)
	}
	if got := connections.WorkerReader.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("worker reader max connections = %d, want 1", got)
	}

	readers := []struct {
		name string
		db   *sql.DB
	}{
		{name: "viewer", db: connections.ViewerReader},
		{name: "admin", db: connections.AdminReader},
		{name: "worker", db: connections.WorkerReader},
	}
	for _, reader := range readers {
		t.Run(reader.name, func(t *testing.T) {
			physicalConnections := make([]*sql.Conn, 0, reader.db.Stats().MaxOpenConnections)
			defer func() {
				for _, connection := range physicalConnections {
					_ = connection.Close()
				}
			}()
			for range reader.db.Stats().MaxOpenConnections {
				connection, err := reader.db.Conn(ctx)
				if err != nil {
					t.Fatal(err)
				}
				physicalConnections = append(physicalConnections, connection)
			}
			for index, connection := range physicalConnections {
				var queryOnly, foreignKeys, busyTimeout int
				if err := connection.QueryRowContext(ctx, `
SELECT (SELECT query_only FROM pragma_query_only),
       (SELECT foreign_keys FROM pragma_foreign_keys),
       (SELECT timeout FROM pragma_busy_timeout)`).Scan(&queryOnly, &foreignKeys, &busyTimeout); err != nil {
					t.Fatal(err)
				}
				if queryOnly != 1 || foreignKeys != 1 || busyTimeout != 5000 {
					t.Fatalf("connection %d has unexpected pragmas: query_only=%d foreign_keys=%d busy_timeout=%d", index, queryOnly, foreignKeys, busyTimeout)
				}
				if _, err := connection.ExecContext(ctx, `INSERT INTO users(username, password_hash, created_at) VALUES ('forbidden', 'x', 1)`); err == nil {
					t.Fatalf("read pool connection %d unexpectedly accepted a write", index)
				}
			}
		})
	}
}

func TestConnectionsRoutesReadsWithoutCrossPoolQueueing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	connections, err := database.OpenConnections(ctx, filepath.Join(t.TempDir(), "routing.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connections.Close() })

	// Pin the only Admin and Worker connections. Their queries must wait, while
	// a Viewer query still uses its independent pool and completes immediately.
	pinnedAdmin, err := connections.AdminReader.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pinnedAdmin.Close()
	pinnedWorker, err := connections.WorkerReader.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pinnedWorker.Close()

	adminCtx, cancelAdmin := context.WithTimeout(database.WithReadWorkload(ctx, database.ReadAdmin), 100*time.Millisecond)
	defer cancelAdmin()
	adminErr := make(chan error, 1)
	go func() {
		var value int
		adminErr <- connections.QueryRowContext(adminCtx, "SELECT 1").Scan(&value)
	}()
	workerCtx, cancelWorker := context.WithTimeout(database.WithReadWorkload(ctx, database.ReadWorker), 100*time.Millisecond)
	defer cancelWorker()
	workerErr := make(chan error, 1)
	go func() {
		var value int
		workerErr <- connections.QueryRowContext(workerCtx, "SELECT 1").Scan(&value)
	}()

	viewerCtx, cancelViewer := context.WithTimeout(database.WithReadWorkload(ctx, database.ReadViewer), time.Second)
	defer cancelViewer()
	var value int
	if err := connections.QueryRowContext(viewerCtx, "SELECT 1").Scan(&value); err != nil {
		t.Fatalf("viewer query was affected by exhausted admin pool: %v", err)
	}
	if value != 1 {
		t.Fatalf("viewer query returned %d, want 1", value)
	}
	if err := <-adminErr; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("admin query error = %v, want deadline exceeded while its pool is exhausted", err)
	}
	if err := <-workerErr; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("worker query error = %v, want deadline exceeded while its pool is exhausted", err)
	}
}

func TestConnectionsReadersRemainAvailableDuringWriterTransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	connections, err := database.OpenConnections(ctx, filepath.Join(t.TempDir(), "wal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connections.Close() })

	viewerWorkload := database.WithReadWorkload(ctx, database.ReadViewer)
	if _, err := connections.ExecContext(viewerWorkload, `INSERT INTO users(username, password_hash, created_at) VALUES ('committed', 'x', 1)`); err != nil {
		t.Fatal(err)
	}
	tx, err := connections.BeginTx(viewerWorkload, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO users(username, password_hash, created_at) VALUES ('pending', 'x', 2)`); err != nil {
		t.Fatal(err)
	}

	viewerCtx, cancel := context.WithTimeout(database.WithReadWorkload(ctx, database.ReadViewer), time.Second)
	defer cancel()
	var count int
	if err := connections.QueryRowContext(viewerCtx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("viewer read failed while writer transaction was open: %v", err)
	}
	if count != 1 {
		t.Fatalf("viewer observed %d rows before commit, want committed snapshot with 1", count)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := connections.QueryRowContext(viewerCtx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("viewer observed %d rows after commit, want 2", count)
	}
}

func TestConnectionsWriterRemainsAvailableDuringReaderSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	connections, err := database.OpenConnections(ctx, filepath.Join(t.TempDir(), "reader-snapshot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connections.Close() })

	readerTx, err := connections.AdminReader.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer readerTx.Rollback()
	var before int
	if err := readerTx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&before); err != nil {
		t.Fatal(err)
	}

	writeCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := connections.ExecContext(writeCtx, `INSERT INTO users(username, password_hash, created_at) VALUES ('writer-during-reader', 'x', 1)`); err != nil {
		t.Fatalf("writer was blocked by reader snapshot: %v", err)
	}
	var during int
	if err := readerTx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&during); err != nil {
		t.Fatal(err)
	}
	if during != before {
		t.Fatalf("reader snapshot changed from %d rows to %d rows", before, during)
	}
	if err := readerTx.Commit(); err != nil {
		t.Fatal(err)
	}

	adminCtx := database.WithReadWorkload(ctx, database.ReadAdmin)
	var after int
	if err := connections.QueryRowContext(adminCtx, "SELECT COUNT(*) FROM users").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Fatalf("fresh reader observed %d rows, want %d", after, before+1)
	}
}
