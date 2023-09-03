package xraysql

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// fakeConnExt is an implementation of [database/sql/driver.Conn],
// and it also implements optional interfaces [database/sql/driver.Execer] and [database/sql/driver.Queryer].
type fakeConnExt fakeConn

// fakeStmtExt is an implementation of [database/sql/driver.Stmt],
// and it also implements optional interface [database/sql/driver.ColumnConverter].
type fakeStmtExt fakeStmt

var _ driver.Conn = &fakeConnExt{}
var _ driver.Execer = &fakeConnExt{}
var _ driver.Queryer = &fakeConnExt{}
var _ driver.Stmt = &fakeStmtExt{}
var _ driver.ColumnConverter = &fakeStmtExt{}

func (c *fakeConnExt) Prepare(query string) (driver.Stmt, error) {
	c.db.printf("[Conn.Prepare] %s", query)
	return &fakeStmtExt{
		db:    c.db,
		query: query,
	}, nil
}

func (c *fakeConnExt) Close() error {
	return nil
}

func (c *fakeConnExt) Begin() (driver.Tx, error) {
	c.db.printf("[Conn.Begin]")
	return &fakeTx{
		db:  c.db,
		opt: c.opt,
	}, nil
}

func (c *fakeConnExt) Exec(query string, args []driver.Value) (driver.Result, error) {
	if strings.Contains(query, "?") {
		// the query contains placeholders, so we can't use this fast-path.
		return nil, driver.ErrSkip
	}

	c.db.printf("[Conn.Exec] %s %s", query, convertValuesToString(args))
	var expect *ExpectExec
	if err := c.db.fetchExpected(&expect); err != nil {
		return nil, err
	}
	if query != expect.Query {
		return nil, fmt.Errorf("unexpected query: want %q, got %q", expect.Query, query)
	}
	if expect.Err != nil {
		return nil, expect.Err
	}
	return &fakeResult{
		lastInsertID: expect.LastInsertID,
		rowsAffected: expect.RowsAffected,
	}, nil
}

func (c *fakeConnExt) Query(query string, args []driver.Value) (driver.Rows, error) {
	if strings.Contains(query, "?") {
		// the query contains placeholders, so we can't use this fast-path.
		return nil, driver.ErrSkip
	}

	c.db.printf("[Conn.Query] %s %s", query, convertValuesToString(args))
	var expect *ExpectQuery
	if err := c.db.fetchExpected(&expect); err != nil {
		return nil, err
	}
	if query != expect.Query {
		return nil, fmt.Errorf("unexpected query: want %q, got %q", expect.Query, query)
	}
	if expect.Err != nil {
		return nil, expect.Err
	}
	return &fakeRows{
		columns: expect.Columns,
		rows:    expect.Rows,
	}, nil
}

func (stmt *fakeStmtExt) Close() error {
	stmt.db.printf("[Stmt.Close]")
	return nil
}

func (stmt *fakeStmtExt) NumInput() int {
	return (*fakeStmt)(stmt).NumInput()
}

func (stmt *fakeStmtExt) Exec(args []driver.Value) (driver.Result, error) {
	return (*fakeStmt)(stmt).Exec(args)
}

func (stmt *fakeStmtExt) Query(args []driver.Value) (driver.Rows, error) {
	return (*fakeStmt)(stmt).Query(args)
}

func (stmt *fakeStmtExt) ColumnConverter(idx int) driver.ValueConverter {
	stmt.db.printf("[Stmt.ColumnConverter] %d", idx)
	return driver.DefaultParameterConverter
}
