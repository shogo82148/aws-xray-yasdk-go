package xraysql

import (
	"context"
	"database/sql/driver"
	"errors"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
)

type driverTx struct {
	driver.Tx
	ctx  context.Context
	seg  *xray.Segment
	conn *driverConn
}

func (tx *driverTx) Commit() error {
	err := xray.Capture(tx.ctx, tx.conn.attr.name, func(ctx context.Context) error {
		tx.conn.attr.populate(ctx, "COMMIT")
		return tx.Tx.Commit()
	})
	tx.seg.Close()
	tx.conn.tx = nil
	return err
}

func (tx *driverTx) Rollback() error {
	err := xray.Capture(tx.ctx, tx.conn.attr.name, func(ctx context.Context) error {
		tx.conn.attr.populate(ctx, "ROLLBACK")
		return tx.Tx.Rollback()
	})
	tx.seg.AddError(errors.New("rollback"))
	tx.seg.Close()
	tx.conn.tx = nil
	return err
}
