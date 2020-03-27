package xraysql

import (
	"context"
	"database/sql/driver"
)

// fakeConnCtx is fakeConn with context support
type fakeConnCtx fakeConn

// fakeStmtCtx is fakeStmt with context
type fakeStmtCtx fakeStmt

var _ driver.Conn = &fakeConnCtx{}
var _ driver.Execer = &fakeConnCtx{}
var _ driver.ExecerContext = &fakeConnCtx{}
var _ driver.Queryer = &fakeConnCtx{}
var _ driver.QueryerContext = &fakeConnCtx{}
var _ driver.ConnBeginTx = &fakeConnCtx{}
var _ driver.ConnPrepareContext = &fakeConnCtx{}
var _ driver.Pinger = &fakeConnCtx{}
var _ driver.Stmt = &fakeStmtCtx{}
var _ driver.ColumnConverter = &fakeStmtCtx{}
var _ driver.StmtExecContext = &fakeStmtCtx{}
var _ driver.StmtQueryContext = &fakeStmtCtx{}

func (c *fakeConnCtx) Ping(ctx context.Context) error {
	c.db.printf("[Conn.Ping]")
	return nil
}

func (c *fakeConnCtx) Prepare(query string) (driver.Stmt, error) {
	panic("not supported")
}

func (c *fakeConnCtx) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	c.db.printf("[Conn.PrepareContext] %s", query)
	return &fakeStmtCtx{
		db:  c.db,
		opt: c.opt,
	}, nil
}

func (c *fakeConnCtx) Close() error {
	return nil
}

func (c *fakeConnCtx) Begin() (driver.Tx, error) {
	panic("not supported")
}

func (c *fakeConnCtx) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.db.printf("[Conn.BeginTx]")
	return &fakeTx{
		db:  c.db,
		opt: c.opt,
	}, nil
}

func (c *fakeConnCtx) Exec(query string, args []driver.Value) (driver.Result, error) {
	panic("not supported")
}

func (c *fakeConnCtx) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.db.printf("[Conn.ExecContext] %s %s", query, convertNamedValuesToString(args))
	return nil, nil
}

func (c *fakeConnCtx) Query(query string, args []driver.Value) (driver.Rows, error) {
	panic("not supported")
}

func (c *fakeConnCtx) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.db.printf("[Conn.QueryContext] %s %s", query, convertNamedValuesToString(args))
	return &fakeRows{}, nil
}

func (stmt *fakeStmtCtx) Close() error {
	stmt.db.printf("[Stmt.Close]")
	return nil
}

func (stmt *fakeStmtCtx) NumInput() int {
	return -1 // fakeDriver doesn't know its number of placeholders
}

func (stmt *fakeStmtCtx) Exec(args []driver.Value) (driver.Result, error) {
	panic("not supported")
}

func (stmt *fakeStmtCtx) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	stmt.db.printf("[Conn.ExecContext] %s", convertNamedValuesToString(args))
	return nil, nil
}

func (stmt *fakeStmtCtx) Query(args []driver.Value) (driver.Rows, error) {
	panic("not supported")
}

func (stmt *fakeStmtCtx) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	stmt.db.printf("[Conn.QueryContext] %s", convertNamedValuesToString(args))
	return &fakeRows{}, nil
}

func (stmt *fakeStmtCtx) ColumnConverter(idx int) driver.ValueConverter {
	stmt.db.printf("[Stmt.ColumnConverter] %d", idx)
	return driver.DefaultParameterConverter
}
