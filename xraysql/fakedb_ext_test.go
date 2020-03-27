package xraysql

import (
	"database/sql/driver"
	"fmt"
	"reflect"
)

// fakeConnExt implements Execer and Queryer
type fakeConnExt fakeConn

// fakeStmtExt implements ColumnConverter
type fakeStmtExt fakeStmt

var _ driver.Conn = &fakeConnExt{}
var _ driver.Execer = &fakeConnExt{}
var _ driver.Queryer = &fakeConnExt{}
var _ driver.Stmt = &fakeStmtExt{}
var _ driver.ColumnConverter = &fakeStmtExt{}

// fetch next expected action.
// v should be a pointer.
func (c *fakeConnExt) fetchExpected(v interface{}) error {
	ptr := reflect.ValueOf(v)
	if ptr.Kind() != reflect.Ptr || ptr.IsNil() {
		return fmt.Errorf("unsupported type: %v", ptr.Type())
	}
	ptr = ptr.Elem()
	if len(c.expect) == 0 {
		return fmt.Errorf("unexpected execution: want %v, got none", ptr.Type())
	}
	expect := reflect.ValueOf(c.expect[0])
	c.expect = c.expect[1:]
	if ptr.Type() != expect.Type() {
		return fmt.Errorf("unexpected execution: want %v, got %v", ptr.Type(), expect.Type())
	}
	ptr.Set(expect)
	return nil
}

func (c *fakeConnExt) Prepare(query string) (driver.Stmt, error) {
	c.db.printf("[Conn.Prepare] %s", query)
	var expect *ExpectQuery
	if err := c.fetchExpected(&expect); err != nil {
		return nil, err
	}
	return &fakeStmtExt{
		db:    c.db,
		opt:   c.opt,
		query: expect,
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
