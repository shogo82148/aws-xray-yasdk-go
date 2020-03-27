package xraysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
)

var muOptionPool sync.RWMutex
var optionPool map[string]*FakeConnOption

func AddOption(opt *FakeConnOption) (dsn string) {
	name := fmt.Sprintf("%p", opt)
	muOptionPool.Lock()
	defer muOptionPool.Unlock()
	if optionPool == nil {
		optionPool = make(map[string]*FakeConnOption)
	}
	optionPool[name] = opt
	return name
}

type FakeExpect interface{}

type ExpectQuery struct {
	Query   string
	Err     error
	Columns []string
	Rows    [][]driver.Value
}

// fakeConnOption is options for fake database.
type FakeConnOption struct {
	// name is the name of database
	Name string

	// ConnType enhances the driver implementation
	ConnType string

	Expect []FakeExpect
}

func init() {
	sql.Register("fakedb", fdriver)
}

type fakeDriver struct {
	mu  sync.Mutex
	dbs map[string]*fakeDB
}

var fdriver = &fakeDriver{}
var _ driver.Driver = fdriver

type fakeDB struct {
	mu  sync.Mutex
	log []string
}

func (d *fakeDriver) Open(name string) (driver.Conn, error) {
	connector, err := (*fakeDriverCtx)(d).OpenConnector(name)
	if err != nil {
		return nil, err
	}
	return connector.Connect(context.Background())
}

// fakeConn is minimum implementation of driver.Conn
type fakeConn struct {
	db     *fakeDB
	opt    *FakeConnOption
	expect []FakeExpect
}

// fakeTx is a fake transaction.
type fakeTx struct {
	db  *fakeDB
	opt *FakeConnOption
}

// fakeStmt is minimum implementation of driver.Stmt
type fakeStmt struct {
	db    *fakeDB
	opt   *FakeConnOption
	query *ExpectQuery
}

type fakeRows struct {
	idx     int
	columns []string
	rows    [][]driver.Value
}

var _ driver.Conn = &fakeConn{}
var _ driver.Tx = &fakeTx{}
var _ driver.Stmt = &fakeStmt{}
var _ driver.Rows = &fakeRows{}

// printf write the params to the log.
func (db *fakeDB) printf(format string, params ...interface{}) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.log = append(db.log, fmt.Sprintf(format, params...))
}

// Log returns the snapshot of log.
func (db *fakeDB) Log() []string {
	db.mu.Lock()
	defer db.mu.Unlock()
	return append([]string(nil), db.log...)
}

// fetch next expected action.
// v should be a pointer.
func (c *fakeConn) fetchExpected(v interface{}) error {
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

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	c.db.printf("[Conn.Prepare] %s", query)
	var expect *ExpectQuery
	if err := c.fetchExpected(&expect); err != nil {
		return nil, err
	}
	return &fakeStmt{
		db:    c.db,
		opt:   c.opt,
		query: expect,
	}, nil
}

func (c *fakeConn) Close() error {
	return nil
}

func (c *fakeConn) Begin() (driver.Tx, error) {
	c.db.printf("[Conn.Begin]")
	return &fakeTx{
		db:  c.db,
		opt: c.opt,
	}, nil
}

func (tx *fakeTx) Commit() error {
	tx.db.printf("[Tx.Commit]")
	return nil
}

func (tx *fakeTx) Rollback() error {
	tx.db.printf("[Tx.Rollback]")
	return nil
}

func (stmt *fakeStmt) Close() error {
	stmt.db.printf("[Stmt.Close]")
	return nil
}

func (stmt *fakeStmt) NumInput() int {
	return -1 // fakeDriver doesn't know its number of placeholders
}

func (stmt *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	stmt.db.printf("[Stmt.Exec] %s", convertValuesToString(args))
	return nil, nil
}

func (stmt *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	stmt.db.printf("[Stmt.Query] %s", convertValuesToString(args))
	if stmt.query == nil {
		return nil, errors.New("expected Exec, but got Query")
	}
	if stmt.query.Err != nil {
		return nil, stmt.query.Err
	}
	return &fakeRows{
		columns: stmt.query.Columns,
		rows:    stmt.query.Rows,
	}, nil
}

func (rows *fakeRows) Columns() []string {
	return rows.columns
}

func (rows *fakeRows) Close() error {
	return nil
}

func (rows *fakeRows) Next(dest []driver.Value) error {
	if rows.idx >= len(rows.rows) {
		return io.EOF
	}
	for i, v := range rows.rows[rows.idx] {
		dest[i] = v
	}
	rows.idx++
	return nil
}
