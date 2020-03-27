package xraysql

import "database/sql/driver"

// fakeConnExt implements Execer and Queryer
type fakeConnExt fakeConn

// fakeStmtExt implements ColumnConverter
type fakeStmtExt fakeStmt

var _ driver.Conn = &fakeConnExt{}
var _ driver.Execer = &fakeConnExt{}
var _ driver.Queryer = &fakeConnExt{}
var _ driver.Stmt = &fakeStmtExt{}
var _ driver.ColumnConverter = &fakeStmtExt{}

func (c *fakeConnExt) Prepare(query string) (driver.Stmt, error) {
	c.db.printf("[Conn.Prepare] %s", query)
	return &fakeStmtExt{
		db:  c.db,
		opt: c.opt,
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
	c.db.printf("[Conn.Exec] %s %s", query, convertValuesToString(args))
	return nil, nil
}

func (c *fakeConnExt) Query(query string, args []driver.Value) (driver.Rows, error) {
	c.db.printf("[Conn.Query] %s %s", query, convertValuesToString(args))
	return &fakeRows{}, nil
}

func (stmt *fakeStmtExt) Close() error {
	stmt.db.printf("[Stmt.Close]")
	return nil
}

func (stmt *fakeStmtExt) NumInput() int {
	return -1 // fakeDriver doesn't know its number of placeholders
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
