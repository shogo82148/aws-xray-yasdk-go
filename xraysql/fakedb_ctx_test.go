package xraysql

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func init() {
	sql.Register("fakedbctx", fdriverctx)
}

func (d *fakeDriverCtx) Open(name string) (driver.Conn, error) {
	return nil, errors.New("not implemented")
}

func (d *fakeDriverCtx) OpenConnector(name string) (driver.Connector, error) {
	var opt *FakeConnOption
	err := json.Unmarshal([]byte(name), &opt)
	if err != nil {
		muOptionPool.RLock()
		opt = optionPool[name]
		muOptionPool.RUnlock()
		if opt == nil {
			return nil, err
		}
	}
	return d.OpenConnectorWithOption(opt)
}

// fakeDriverCtx is fakeDriver with context support.
type fakeDriverCtx fakeDriver

// fakeConnector is fakeConnector with context support.
type fakeConnector struct {
	driver *fakeDriverCtx
	opt    *FakeConnOption
	db     *fakeDB
}

var fdriverctx = &fakeDriverCtx{}
var _ driver.DriverContext = fdriverctx
var _ driver.Connector = &fakeConnector{}

func (d *fakeDriverCtx) OpenConnectorWithOption(opt *FakeConnOption) (driver.Connector, error) {
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
			log:    []string{},
			expect: opt.Expect,
		}
		if d.dbs == nil {
			d.dbs = make(map[string]*fakeDB)
		}
		d.dbs[opt.Name] = db
	}

	return &fakeConnector{
		driver: d,
		opt:    opt,
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
			db:  c.db,
			opt: c.opt,
		}
	case "fakeConnExt":
		conn = &fakeConnExt{
			db:  c.db,
			opt: c.opt,
		}
	case "fakeConnCtx":
		conn = &fakeConnCtx{
			db:  c.db,
			opt: c.opt,
		}
	default:
		return nil, errors.New("known ConnType")
	}

	return conn, nil
}

func (c *fakeConnector) Driver() driver.Driver {
	return c.driver
}

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
		db:    c.db,
		query: query,
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
	if strings.Contains(query, "?") {
		// the query contains placeholders, so we can't use this fast-path.
		return nil, driver.ErrSkip
	}

	c.db.printf("[Conn.ExecContext] %s %s", query, convertNamedValuesToString(args))
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

func (c *fakeConnCtx) Query(query string, args []driver.Value) (driver.Rows, error) {
	panic("not supported")
}

func (c *fakeConnCtx) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "?") {
		// the query contains placeholders, so we can't use this fast-path.
		return nil, driver.ErrSkip
	}

	c.db.printf("[Conn.QueryContext] %s %s", query, convertNamedValuesToString(args))
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

func (stmt *fakeStmtCtx) Query(args []driver.Value) (driver.Rows, error) {
	panic("not supported")
}

func (stmt *fakeStmtCtx) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	stmt.db.printf("[Stmt.QueryContext] %s", convertNamedValuesToString(args))
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

func (stmt *fakeStmtCtx) ColumnConverter(idx int) driver.ValueConverter {
	stmt.db.printf("[Stmt.ColumnConverter] %d", idx)
	return driver.DefaultParameterConverter
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
