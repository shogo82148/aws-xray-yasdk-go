package xraysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
)

type driverConn struct {
	driver.Conn
	attr *dbAttribute
	tx   *driverTx
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
	seg.AddMetadata("tx_options", map[string]interface{}{
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

// util function for handling a transaction segment.
func (conn *driverConn) beginSubsegment(ctx context.Context) (context.Context, *xray.Segment) {
	parent := ctx
	if conn.tx != nil {
		parent = conn.tx.ctx
	}
	_, seg := xray.BeginSubsegment(parent, conn.attr.name)
	return xray.WithSegment(ctx, seg), seg
}

func (conn *driverConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	panic("not supported")
}

func (conn *driverConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := conn.Conn.(driver.Execer)
	if !ok {
		return nil, driver.ErrSkip
	}

	ctx, seg := conn.beginSubsegment(ctx)
	defer seg.Close()

	var err error
	var result driver.Result
	if execerCtx, ok := conn.Conn.(driver.ExecerContext); ok {
		result, err = execerCtx.ExecContext(ctx, query, args)
	} else {
		select {
		default:
		case <-ctx.Done():
			err := ctx.Err()
			seg.AddError(err)
			return nil, err
		}
		dargs, err0 := namedValuesToValues(args)
		if err0 != nil {
			seg.AddError(err0)
			return nil, err0
		}
		result, err = execer.Exec(query, dargs)
	}

	if err == driver.ErrSkip {
		conn.attr.populate(ctx, query+msgErrSkip)
		return nil, driver.ErrSkip
	}
	conn.attr.populate(ctx, query)
	seg.AddError(err)
	return result, err
}

func (conn *driverConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	panic("not supported")
}

func (conn *driverConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := conn.Conn.(driver.Queryer)
	if !ok {
		return nil, driver.ErrSkip
	}

	ctx, seg := conn.beginSubsegment(ctx)
	defer seg.Close()

	var err error
	var rows driver.Rows
	if queryerCtx, ok := conn.Conn.(driver.QueryerContext); ok {
		rows, err = queryerCtx.QueryContext(ctx, query, args)
	} else {
		select {
		default:
		case <-ctx.Done():
			err := ctx.Err()
			seg.AddError(err)
			return nil, err
		}
		dargs, err0 := namedValuesToValues(args)
		if err0 != nil {
			seg.AddError(err0)
			return nil, err0
		}
		rows, err = queryer.Query(query, dargs)
	}

	if err == driver.ErrSkip {
		conn.attr.populate(ctx, query+msgErrSkip)
		return nil, driver.ErrSkip
	}
	conn.attr.populate(ctx, query)
	seg.AddError(err)
	return rows, nil
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
