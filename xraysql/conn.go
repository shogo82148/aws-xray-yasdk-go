package xraysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
)

type driverConn struct {
	// Conn is the original connection.
	driver.Conn

	// attr is the attributes of the connection.
	attr *dbAttribute

	// tx is the current transaction.
	tx *driverTx

	// seg is the segment for the connection.
	seg *xray.Segment
}

func (d *driverDriver) Open(dataSourceName string) (driver.Conn, error) {
	panic("not supported")
}

func (conn *driverConn) Ping(ctx context.Context) error {
	return xray.Capture(ctx, conn.attr.name, func(ctx context.Context) error {
		conn.attr.populate(ctx, "PING")
		if p, ok := conn.Conn.(driver.Pinger); ok {
			return p.Ping(ctx)
		}
		return nil
	})
}

func (conn *driverConn) Prepare(query string) (driver.Stmt, error) {
	panic("not supported")
}

func (conn *driverConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	var stmt driver.Stmt
	var err error
	if connCtx, ok := conn.Conn.(driver.ConnPrepareContext); ok {
		stmt, err = connCtx.PrepareContext(ctx, query)
	} else {
		stmt, err = conn.Conn.Prepare(query)
		if err == nil {
			select {
			default:
			case <-ctx.Done():
				stmt.Close()
				return nil, ctx.Err()
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return &driverStmt{
		Stmt:  stmt,
		query: query,
		conn:  conn,
	}, nil
}

func (conn *driverConn) Begin() (driver.Tx, error) {
	panic("not supported")
}

func (conn *driverConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	ctx, seg := xray.BeginSubsegment(ctx, "transaction")
	seg.AddMetadataToNamespace("sql", "tx_options", map[string]any{
		"isolation_level": sql.IsolationLevel(opts.Isolation).String(),
		"read_only":       opts.ReadOnly,
	})

	var tx driver.Tx
	err := xray.Capture(ctx, conn.attr.name, func(ctx context.Context) error {
		var err error
		conn.attr.populate(ctx, "BEGIN")
		if connCtx, ok := conn.Conn.(driver.ConnBeginTx); ok {
			tx, err = connCtx.BeginTx(ctx, opts)
		} else {
			if opts.Isolation != driver.IsolationLevel(sql.LevelDefault) {
				return errors.New("xraysql: driver does not support non-default isolation level")
			}
			if opts.ReadOnly {
				return errors.New("xraysql: driver does not support read-only transactions")
			}
			tx, err = conn.Conn.Begin()
			if err == nil {
				select {
				default:
				case <-ctx.Done():
					tx.Rollback()
					return ctx.Err()
				}
			}
		}
		return err
	})
	if err != nil {
		seg.SetFault()
		seg.Close()
		return nil, err
	}
	tx1 := &driverTx{
		Tx:   tx,
		ctx:  ctx,
		seg:  seg,
		conn: conn,
	}
	conn.tx = tx1
	return tx1, nil
}

// beginSubsegment begins a new sub-segment if there is no segment.
func (conn *driverConn) beginSubsegment(ctx context.Context) (context.Context, *xray.Segment) {
	if conn.seg != nil {
		return xray.WithSegment(ctx, conn.seg), conn.seg
	}

	parent := ctx
	if conn.tx != nil {
		// if there is a transaction, use the transaction's segment as the parent.
		parent = conn.tx.ctx
	}
	_, seg := xray.BeginSubsegment(parent, conn.attr.name)
	conn.seg = seg
	return xray.WithSegment(ctx, seg), seg
}

// closeSubsegment closes the current sub-segment.
func (conn *driverConn) closeSubsegment() {
	if conn.seg != nil {
		conn.seg.Close()
		conn.seg = nil
	}
}

func (conn *driverConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	panic("not supported")
}

func (conn *driverConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := conn.Conn.(driver.Execer)
	execerCtx, okCtx := conn.Conn.(driver.ExecerContext)
	if !ok && !okCtx {
		return nil, driver.ErrSkip
	}

	ctx, seg := conn.beginSubsegment(ctx)

	var err error
	var result driver.Result
	if okCtx {
		result, err = execerCtx.ExecContext(ctx, query, args)
	} else {
		select {
		default:
		case <-ctx.Done():
			err := ctx.Err()
			seg.AddError(err)
			conn.closeSubsegment()
			return nil, err
		}
		dargs, err0 := namedValuesToValues(args)
		if err0 != nil {
			seg.AddError(err0)
			conn.closeSubsegment()
			return nil, err0
		}
		result, err = execer.Exec(query, dargs)
	}

	if err == driver.ErrSkip {
		return nil, driver.ErrSkip
	}
	conn.attr.populateToSegment(seg, query)
	seg.AddError(err)
	conn.closeSubsegment()
	return result, err
}

func (conn *driverConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	panic("not supported")
}

func (conn *driverConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := conn.Conn.(driver.Queryer)
	queryerCtx, okCtx := conn.Conn.(driver.QueryerContext)
	if !ok && !okCtx {
		return nil, driver.ErrSkip
	}

	ctx, seg := conn.beginSubsegment(ctx)

	var err error
	var rows driver.Rows
	if okCtx {
		rows, err = queryerCtx.QueryContext(ctx, query, args)
	} else {
		select {
		default:
		case <-ctx.Done():
			err := ctx.Err()
			seg.AddError(err)
			conn.closeSubsegment()
			return nil, err
		}
		dargs, err0 := namedValuesToValues(args)
		if err0 != nil {
			seg.AddError(err0)
			conn.closeSubsegment()
			return nil, err0
		}
		rows, err = queryer.Query(query, dargs)
	}

	if err == driver.ErrSkip {
		return nil, driver.ErrSkip
	}
	conn.attr.populate(ctx, query)
	seg.AddError(err)
	conn.closeSubsegment()
	return rows, err
}

func (conn *driverConn) Close() error {
	conn.closeSubsegment()
	return conn.Conn.Close()
}

func (conn *driverConn) ResetSession(ctx context.Context) error {
	conn.closeSubsegment()
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

// the same as driver.Validator that is available from Go 1.15 or later.
// Copied from database/sql/driver/driver.go for supporting old Go versions.
type validator interface {
	// IsValid is called prior to placing the connection into the
	// connection pool. The connection will be discarded if false is returned.
	IsValid() bool
}

// IsValid implements driver.Validator.
// It calls the IsValid method of the original connection.
// If the original connection does not satisfy "database/sql/driver".Validator, it always returns true.
func (conn *driverConn) IsValid() bool {
	if v, ok := conn.Conn.(validator); ok {
		return v.IsValid()
	}
	return true
}
