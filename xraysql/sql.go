package xraysql

import (
	"database/sql"
	"database/sql/driver"
)

// we can't know that the original driver will return driver.ErrSkip in advance.
// so we add this message to the query if it returns driver.ErrSkip.
const msgErrSkip = " -- skip fast-path; continue as if unimplemented"

// Open xxx
func Open(driverName, dataSourceName string) *sql.DB {
	return nil
}

// OpenDB xxx
func OpenDB(c driver.Connector) *sql.DB {
	return nil
}

type driverDriver struct {
	driver.Driver
	baseName string // the name of the base driver
}

func (d *driverDriver) Open(dsn string) (driver.Conn, error) {
	return nil, nil
}
