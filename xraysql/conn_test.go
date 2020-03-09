package xraysql

import "database/sql/driver"

// compile time checking to satisfy the interface
// https://golang.org/doc/effective_go.html#blank_implements
var _ driver.Conn = (*driverConn)(nil)
var _ driver.ConnBeginTx = (*driverConn)(nil)
var _ driver.ConnPrepareContext = (*driverConn)(nil)
var _ driver.NamedValueChecker = (*driverConn)(nil)
var _ driver.SessionResetter = (*driverConn)(nil)
