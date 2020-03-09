package xraysql

import (
	"context"
	"database/sql/driver"
	"errors"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
)

type driverStmt struct {
	driver.Stmt
	conn  *driverConn
	query string
}

// util function for handling a transaction segment.
func (stmt *driverStmt) beginSubsegment(ctx context.Context) (context.Context, *xray.Segment) {
	parent := ctx
	if stmt.conn.tx != nil {
		parent = stmt.conn.tx.ctx
	}
	_, seg := xray.BeginSubsegment(parent, stmt.conn.attr.name)
	return xray.WithSegment(ctx, seg), seg
}

func (stmt *driverStmt) Close() error {
	return stmt.Stmt.Close()
}

func (stmt *driverStmt) NumInput() int {
	return stmt.Stmt.NumInput()
}

func (stmt *driverStmt) Exec(args []driver.Value) (driver.Result, error) {
	panic("not supported")
}

func (stmt *driverStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	var result driver.Result
	ctx, seg := stmt.beginSubsegment(ctx)
	defer seg.Close()
	stmt.conn.attr.populate(ctx, stmt.query)
	var err error
	if execerContext, ok := stmt.Stmt.(driver.StmtExecContext); ok {
		result, err = execerContext.ExecContext(ctx, args)
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
		result, err = stmt.Stmt.Exec(dargs)
	}
	seg.AddError(err)
	return result, err
}

func (stmt *driverStmt) Query(args []driver.Value) (driver.Rows, error) {
	panic("not supported")
}

func (stmt *driverStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	var result driver.Rows
	ctx, seg := stmt.beginSubsegment(ctx)
	defer seg.Close()
	stmt.conn.attr.populate(ctx, stmt.query)
	var err error
	if queryCtx, ok := stmt.Stmt.(driver.StmtQueryContext); ok {
		result, err = queryCtx.QueryContext(ctx, args)
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
		result, err = stmt.Stmt.Query(dargs)
	}
	seg.AddError(err)
	return result, err
}

func (stmt *driverStmt) ColumnConverter(idx int) driver.ValueConverter {
	if conv, ok := stmt.Stmt.(driver.ColumnConverter); ok {
		return conv.ColumnConverter(idx)
	}
	return driver.DefaultParameterConverter
}

// CheckNamedValue for implementing NamedValueChecker
func (stmt *driverStmt) CheckNamedValue(nv *driver.NamedValue) (err error) {
	if nvc, ok := stmt.Stmt.(driver.NamedValueChecker); ok {
		return nvc.CheckNamedValue(nv)
	}
	// When converting data in sql/driver/convert.go, it is checked first whether the `stmt`
	// implements `NamedValueChecker`, and then checks if `conn` implements NamedValueChecker.
	// In the case of "go-sql-proxy", the `proxy.Stmt` "implements" `CheckNamedValue` here,
	// so we also check both `stmt` and `conn` inside here.
	if nvc, ok := stmt.conn.Conn.(driver.NamedValueChecker); ok {
		return nvc.CheckNamedValue(nv)
	}
	// fallback to default
	return defaultCheckNamedValue(nv)
}

func namedValuesToValues(args []driver.NamedValue) ([]driver.Value, error) {
	var err error
	ret := make([]driver.Value, len(args))
	for _, arg := range args {
		if len(arg.Name) > 0 {
			err = errors.New("xraysql: driver does not support the use of Named Parameters")
		}
		ret[arg.Ordinal-1] = arg.Value
	}
	return ret, err
}
