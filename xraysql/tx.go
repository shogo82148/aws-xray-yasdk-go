package xraysql

import "database/sql/driver"

type driverTx struct {
	driver.Tx
}

func (tx *driverTx) Commit() error {
	return tx.Tx.Commit()
}

func (tx *driverTx) Rollback() error {
	return tx.Tx.Rollback()
}
