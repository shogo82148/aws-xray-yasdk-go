package xraysql

import (
	"context"
	"database/sql/driver"
)

type driverConn struct {
	driver.Conn
}

func (conn *driverConn) Ping(ctx context.Context) error {
	return nil
}

func (conn *driverConn) Prepare(query string) (driver.Stmt, error) {
	panic("not supported")
}

func (conn *driverConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return nil, nil
}

func (conn *driverConn) Begin() (driver.Tx, error) {
	panic("not supported")
}

func (conn *driverConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return nil, nil
}

func (conn *driverConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	panic("not supported")
}

func (conn *driverConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return nil, nil
}

func (conn *driverConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	panic("not supported")
}

func (conn *driverConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return nil, nil
}

func (conn *driverConn) Close() error {
	return conn.Conn.Close()
}

func (conn *driverConn) ResetSession(ctx context.Context) error {
	if sr, ok := conn.Conn.(driver.SessionResetter); ok {
		return sr.ResetSession(ctx)
	}
	return nil
}

// copied from https://github.com/golang/go/blob/e6ebbe0d20fe877b111cf4ccf8349cba129d6d3a/src/database/sql/convert.go#L93-L99
// defaultCheckNamedValue wraps the default ColumnConverter to have the same
// function signature as the CheckNamedValue in the driver.NamedValueChecker
// interface.
func defaultCheckNamedValue(nv *driver.NamedValue) (err error) {
	nv.Value, err = driver.DefaultParameterConverter.ConvertValue(nv.Value)
	return err
}

// CheckNamedValue for implementing driver.NamedValueChecker
// This function may be unnecessary because `proxy.Stmt` already implements `NamedValueChecker`,
// but it is implemented just in case.
func (conn *driverConn) CheckNamedValue(nv *driver.NamedValue) (err error) {
	if nvc, ok := conn.Conn.(driver.NamedValueChecker); ok {
		return nvc.CheckNamedValue(nv)
	}
	// fallback to default
	return defaultCheckNamedValue(nv)
}
