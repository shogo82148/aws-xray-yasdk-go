package xraysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
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

type FakeExpect any

type ExpectQuery struct {
	Query   string
	Err     error
	Args    []driver.NamedValue
	Columns []string
	Rows    [][]driver.Value
}

type ExpectExec struct {
	Query        string
	Err          error
	Args         []driver.NamedValue
	LastInsertID int64
	RowsAffected int64
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
	mu     sync.Mutex
	log    []string
	expect []FakeExpect
}

func (d *fakeDriver) Open(name string) (driver.Conn, error) {
	connector, err := (*fakeDriverCtx)(d).OpenConnector(name)
	if err != nil {
		return nil, err
	}
	return connector.Connect(context.Background())
}

// fakeConn is minimum implementation of [database/sql/driver.Conn].
type fakeConn struct {
	db  *fakeDB
	opt *FakeConnOption
}

// fakeTx is a fake transaction.
type fakeTx struct {
	db  *fakeDB
	opt *FakeConnOption
}

// fakeStmt is minimum implementation of [database/sql/driver.Stmt].
type fakeStmt struct {
	db    *fakeDB
	query string
}

type fakeResult struct {
	lastInsertID int64
	rowsAffected int64
}

type fakeRows struct {
	idx     int
	columns []string
	rows    [][]driver.Value
}

var _ driver.Conn = &fakeConn{}
var _ driver.Tx = &fakeTx{}
var _ driver.Stmt = &fakeStmt{}
var _ driver.Result = &fakeResult{}
var _ driver.Rows = &fakeRows{}

// printf write the params to the log.
func (db *fakeDB) printf(format string, params ...any) {
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
func (db *fakeDB) fetchExpected(v any) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	ptr := reflect.ValueOf(v)
	if ptr.Kind() != reflect.Ptr || ptr.IsNil() {
		return fmt.Errorf("unsupported type: %v", ptr.Type())
	}
	ptr = ptr.Elem()
	if len(db.expect) == 0 {
		return fmt.Errorf("unexpected execution: want %v, got none", ptr.Type())
	}
	expect := reflect.ValueOf(db.expect[0])
	if ptr.Type() != expect.Type() {
		return fmt.Errorf("unexpected execution: want %v, got %v", ptr.Type(), expect.Type())
	}
	ptr.Set(expect)
	db.expect = db.expect[1:]
	return nil
}

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	c.db.printf("[Conn.Prepare] %s", query)
	return &fakeStmt{
		db:    c.db,
		query: query,
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
	return -1
}

func (stmt *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	stmt.db.printf("[Stmt.Exec] %s", convertValuesToString(args))
	var expect *ExpectExec
	if err := stmt.db.fetchExpected(&expect); err != nil {
		return nil, err
	}
	if stmt.query != expect.Query {
		return nil, fmt.Errorf("unexpected query: want %q, got %q", expect.Query, stmt.query)
	}
	if expect.Err != nil {
		return nil, expect.Err
	}
	return &fakeResult{
		lastInsertID: expect.LastInsertID,
		rowsAffected: expect.RowsAffected,
	}, nil
}

func (stmt *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	stmt.db.printf("[Stmt.Query] %s", convertValuesToString(args))
	var expect *ExpectQuery
	if err := stmt.db.fetchExpected(&expect); err != nil {
		return nil, err
	}
	if stmt.query != expect.Query {
		return nil, fmt.Errorf("unexpected query: want %q, got %q", expect.Query, stmt.query)
	}
	if expect.Err != nil {
		return nil, expect.Err
	}
	return &fakeRows{
		columns: expect.Columns,
		rows:    expect.Rows,
	}, nil
}

func (result *fakeResult) LastInsertId() (int64, error) {
	return result.lastInsertID, nil
}

func (result *fakeResult) RowsAffected() (int64, error) {
	return result.rowsAffected, nil
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
	copy(dest, rows.rows[rows.idx])
	rows.idx++
	return nil
}
