package xraysql

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
)

type fakeExpect interface{}

type ExpectQuery struct {
	Query   string
	Err     error
	Columns []string
	Rows    [][]driver.Value
}

// fakeConnOption is options for fake database.
type fakeConnOption struct {
	// name is the name of database
	Name string

	// ConnType enhances the driver implementation
	ConnType string

	Expect []fakeExpect
}

type fakeDriver struct {
	mu  sync.Mutex
	dbs map[string]*fakeDB
}

type fakeDB struct {
	mu  sync.Mutex
	log []string
}

// fakeConn is minimum implementation of driver.Conn
type fakeConn struct {
	db     *fakeDB
	opt    *fakeConnOption
	expect []fakeExpect
}

// fakeTx is a fake transaction.
type fakeTx struct {
	db  *fakeDB
	opt *fakeConnOption
}

// fakeStmt is minimum implementation of driver.Stmt
type fakeStmt struct {
	db    *fakeDB
	opt   *fakeConnOption
	query *ExpectQuery
}

type fakeRows struct {
	idx     int
	columns []string
	rows    [][]driver.Value
}

var fdriver = &fakeDriver{}
var _ driver.Driver = fdriver
var _ driver.Conn = &fakeConn{}
var _ driver.Tx = &fakeTx{}
var _ driver.Stmt = &fakeStmt{}
var _ driver.Rows = &fakeRows{}

func init() {
	sql.Register("fakedb", fdriver)
}

func (d *fakeDriver) Open(name string) (driver.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var opt fakeConnOption
	err := json.Unmarshal([]byte(name), &opt)
	if err != nil {
		return nil, err
	}

	db, ok := d.dbs[opt.Name]
	if !ok {
		db = &fakeDB{
			log: []string{},
		}
		if d.dbs == nil {
			d.dbs = make(map[string]*fakeDB)
		}
		d.dbs[name] = db
	}

	var conn driver.Conn
	switch opt.ConnType {
	case "", "fakeConn":
		conn = &fakeConn{
			db:     db,
			opt:    &opt,
			expect: opt.Expect,
		}
	case "fakeConnExt":
		conn = &fakeConnExt{
			db:     db,
			opt:    &opt,
			expect: opt.Expect,
		}
	case "fakeConnCtx":
		conn = &fakeConnCtx{
			db:     db,
			opt:    &opt,
			expect: opt.Expect,
		}
	default:
		return nil, errors.New("known ConnType")
	}

	return conn, nil
}

func (d *fakeDriver) DB(name string) *fakeDB {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dbs[name]
}

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

type fakeDriverCtx fakeDriver
type fakeConnector struct {
	driver *fakeDriverCtx
	opt    *fakeConnOption
	db     *fakeDB
}

var fdriverctx = &fakeDriverCtx{}
var _ driver.DriverContext = fdriverctx
var _ driver.Connector = &fakeConnector{}

func init() {
	sql.Register("fakedbctx", fdriverctx)
}

func (d *fakeDriverCtx) Open(name string) (driver.Conn, error) {
	return nil, errors.New("not implemented")
}

func (d *fakeDriverCtx) OpenConnector(name string) (driver.Connector, error) {
	var opt fakeConnOption
	err := json.Unmarshal([]byte(name), &opt)
	if err != nil {
		return nil, err
	}
	return d.OpenConnectorWithOption(opt)
}

func (d *fakeDriverCtx) OpenConnectorWithOption(opt fakeConnOption) (driver.Connector, error) {
	// validate options
	switch opt.ConnType {
	case "", "fakeConn", "fakeConnExt", "fakeConnCtx":
		// validation OK
	default:
		return nil, errors.New("known ConnType")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	db, ok := d.dbs[opt.Name]
	if !ok {
		db = &fakeDB{
			log: []string{},
		}
		if d.dbs == nil {
			d.dbs = make(map[string]*fakeDB)
		}
		d.dbs[opt.Name] = db
	}

	return &fakeConnector{
		driver: d,
		opt:    &opt,
		db:     db,
	}, nil
}

func (d *fakeDriverCtx) DB(name string) *fakeDB {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dbs[name]
}

func (c *fakeConnector) Connect(ctx context.Context) (driver.Conn, error) {
	var conn driver.Conn
	opt := c.opt
	switch opt.ConnType {
	case "", "fakeConn":
		conn = &fakeConn{
			db:     c.db,
			opt:    c.opt,
			expect: opt.Expect,
		}
	case "fakeConnExt":
		conn = &fakeConnExt{
			db:     c.db,
			opt:    c.opt,
			expect: opt.Expect,
		}
	case "fakeConnCtx":
		conn = &fakeConnCtx{
			db:     c.db,
			opt:    c.opt,
			expect: opt.Expect,
		}
	default:
		return nil, errors.New("known ConnType")
	}

	return conn, nil
}

func (c *fakeConnector) Driver() driver.Driver {
	return c.driver
}

func convertValuesToString(args []driver.Value) string {
	buf := new(bytes.Buffer)
	for _, arg := range args {
		fmt.Fprintf(buf, " %#v", arg)
	}
	return buf.String()
}

func convertNamedValuesToString(args []driver.NamedValue) string {
	buf := new(bytes.Buffer)
	for _, arg := range args {
		fmt.Fprintf(buf, " %#v", arg)
	}
	return buf.String()
}
