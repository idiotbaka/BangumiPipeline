package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	defaultViewerReadConnections = 4
	defaultAdminReadConnections  = 1
	defaultWorkerReadConnections = 1
)

// Executor is the database surface used by the application services. A plain
// *sql.DB implements it, which keeps unit tests and one-off tools compatible.
// Connections additionally routes reads to an isolated pool based on context,
// while always sending mutations and transactions to the single writer.
type Executor interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

var (
	_ Executor = (*sql.DB)(nil)
	_ Executor = (*Connections)(nil)
)

type ReadWorkload uint8

const (
	ReadWorker ReadWorkload = iota
	ReadViewer
	ReadAdmin
)

type readWorkloadContextKey struct{}

// WithReadWorkload marks which isolated read pool should serve queries made
// with ctx. Contexts without a marker intentionally use the bounded worker
// pool, which is the safest default for startup and background work.
func WithReadWorkload(ctx context.Context, workload ReadWorkload) context.Context {
	return context.WithValue(ctx, readWorkloadContextKey{}, workload)
}

type Connections struct {
	Writer       *sql.DB
	ViewerReader *sql.DB
	AdminReader  *sql.DB
	WorkerReader *sql.DB
}

// OpenConnections opens the writer first so WAL and migrations are complete
// before any read-only connection is created.
func OpenConnections(ctx context.Context, path string) (*Connections, error) {
	writer, err := Open(ctx, path)
	if err != nil {
		return nil, err
	}
	connections := &Connections{Writer: writer}
	closeOnError := func(openErr error) (*Connections, error) {
		_ = connections.Close()
		return nil, openErr
	}

	if connections.ViewerReader, err = openReadPool(ctx, path, defaultViewerReadConnections); err != nil {
		return closeOnError(fmt.Errorf("open viewer database reader: %w", err))
	}
	if connections.AdminReader, err = openReadPool(ctx, path, defaultAdminReadConnections); err != nil {
		return closeOnError(fmt.Errorf("open admin database reader: %w", err))
	}
	if connections.WorkerReader, err = openReadPool(ctx, path, defaultWorkerReadConnections); err != nil {
		return closeOnError(fmt.Errorf("open worker database reader: %w", err))
	}
	return connections, nil
}

func (c *Connections) BeginTx(ctx context.Context, options *sql.TxOptions) (*sql.Tx, error) {
	return c.Writer.BeginTx(ctx, options)
}

func (c *Connections) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.Writer.ExecContext(ctx, query, args...)
}

func (c *Connections) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.reader(ctx).QueryContext(ctx, query, args...)
}

func (c *Connections) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.reader(ctx).QueryRowContext(ctx, query, args...)
}

func (c *Connections) reader(ctx context.Context) *sql.DB {
	workload, _ := ctx.Value(readWorkloadContextKey{}).(ReadWorkload)
	switch workload {
	case ReadViewer:
		return c.ViewerReader
	case ReadAdmin:
		return c.AdminReader
	default:
		return c.WorkerReader
	}
}

// Close closes readers before the writer. This keeps the WAL owner alive until
// every in-flight read snapshot has been released.
func (c *Connections) Close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	for _, db := range []*sql.DB{c.ViewerReader, c.AdminReader, c.WorkerReader, c.Writer} {
		if db != nil {
			closeErr = errors.Join(closeErr, db.Close())
		}
	}
	return closeErr
}

func openReadPool(ctx context.Context, path string, maxConnections int) (*sql.DB, error) {
	dsn, err := readOnlyDSN(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxConnections)
	db.SetMaxIdleConns(maxConnections)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	var queryOnly int
	if err := db.QueryRowContext(ctx, "PRAGMA query_only").Scan(&queryOnly); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("verify query_only: %w", err)
	}
	if queryOnly != 1 {
		_ = db.Close()
		return nil, errors.New("read pool opened without query_only")
	}
	return db, nil
}

func readOnlyDSN(path string) (string, error) {
	return sqliteFileDSN(path, "ro", []string{
		"foreign_keys(1)",
		"busy_timeout(5000)",
		"query_only(1)",
	})
}

func writeDSN(path string) (string, error) {
	return sqliteFileDSN(path, "rwc", []string{
		"foreign_keys(1)",
		"busy_timeout(5000)",
		"synchronous(NORMAL)",
	})
}

func sqliteFileDSN(path, mode string, pragmas []string) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}
	urlPath := filepath.ToSlash(absolutePath)
	if runtime.GOOS == "windows" && !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	query := url.Values{}
	query.Set("mode", mode)
	for _, pragma := range pragmas {
		query.Add("_pragma", pragma)
	}
	return (&url.URL{Scheme: "file", Path: urlPath, RawQuery: query.Encode()}).String(), nil
}
