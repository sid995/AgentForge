// Package database owns PostgreSQL connectivity and tenant-scoped transaction setup.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/sid995/agentforge/services/platform-api/internal/config"
)

// Pool is the bounded PostgreSQL connection pool used by repository adapters.
type Pool struct {
	pool           *sql.DB
	acquireTimeout time.Duration
}

// TenantTx owns a transaction and its acquired connection. The connection is
// returned to the pool exactly once after commit or rollback.
type TenantTx struct {
	tx   *sql.Tx
	conn *sql.Conn
	once sync.Once
}

// Stats is a stable, dependency-free seam for later database metrics.
type Stats struct {
	OpenConns int
	InUse     int
	Idle      int
}

// Open validates connectivity and returns a configured PostgreSQL pool.
func Open(ctx context.Context, configuration config.DatabaseConfig) (*Pool, error) {
	pool, err := sql.Open("pgx", configuration.URL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL driver: %w", err)
	}
	pool.SetMaxOpenConns(int(configuration.MaxConns))
	pool.SetMaxIdleConns(configuration.MaxIdleConns)
	pool.SetConnMaxLifetime(configuration.MaxConnLifetime)

	connectContext, cancel := context.WithTimeout(ctx, configuration.ConnectTimeout)
	defer cancel()
	database := &Pool{pool: pool, acquireTimeout: configuration.AcquireTimeout}
	if err := database.Ping(connectContext); err != nil {
		_ = pool.Close()
		return nil, err
	}
	return database, nil
}

// Ping checks the live database dependency without exposing connection details.
func (pool *Pool) Ping(ctx context.Context) error {
	context, cancel := context.WithTimeout(ctx, pool.acquireTimeout)
	defer cancel()
	if err := pool.pool.PingContext(context); err != nil {
		return fmt.Errorf("PostgreSQL unavailable: %w", err)
	}
	return nil
}

// BeginTenant starts a transaction and installs transaction-local tenant context.
// The setting is automatically cleared on commit or rollback before a pooled
// connection can be reused.
func (pool *Pool) BeginTenant(ctx context.Context, tenantID uuid.UUID) (*TenantTx, error) {
	acquireContext, cancel := context.WithTimeout(ctx, pool.acquireTimeout)
	defer cancel()
	connection, err := pool.pool.Conn(acquireContext)
	if err != nil {
		return nil, fmt.Errorf("acquire tenant connection: %w", err)
	}
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("begin tenant transaction: %w", err)
	}
	tenantTx := &TenantTx{tx: tx, conn: connection}
	if _, err := tenantTx.ExecContext(ctx, "select set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
		_ = tenantTx.Rollback()
		return nil, fmt.Errorf("set tenant context: %w", err)
	}
	return tenantTx, nil
}

// ExecContext runs a statement inside the tenant-scoped transaction.
func (transaction *TenantTx) ExecContext(ctx context.Context, query string, arguments ...any) (sql.Result, error) {
	return transaction.tx.ExecContext(ctx, query, arguments...)
}

// QueryContext runs a query inside the tenant-scoped transaction.
func (transaction *TenantTx) QueryContext(ctx context.Context, query string, arguments ...any) (*sql.Rows, error) {
	return transaction.tx.QueryContext(ctx, query, arguments...)
}

// QueryRowContext runs a single-row query inside the tenant-scoped transaction.
func (transaction *TenantTx) QueryRowContext(ctx context.Context, query string, arguments ...any) *sql.Row {
	return transaction.tx.QueryRowContext(ctx, query, arguments...)
}

// Commit commits the transaction and returns its connection to the pool.
func (transaction *TenantTx) Commit() error {
	err := transaction.tx.Commit()
	transaction.release()
	return err
}

// Rollback rolls the transaction back and returns its connection to the pool.
func (transaction *TenantTx) Rollback() error {
	err := transaction.tx.Rollback()
	transaction.release()
	return err
}

func (transaction *TenantTx) release() {
	transaction.once.Do(func() { _ = transaction.conn.Close() })
}

// Raw exposes the pool to explicit privileged adapters and integration tests.
func (pool *Pool) Raw() *sql.DB { return pool.pool }

// Close releases all database connections.
func (pool *Pool) Close() { _ = pool.pool.Close() }

// Stats returns bounded connection-pool measurements for future instrumentation.
func (pool *Pool) Stats() Stats {
	statistics := pool.pool.Stats()
	return Stats{OpenConns: statistics.OpenConnections, InUse: statistics.InUse, Idle: statistics.Idle}
}
