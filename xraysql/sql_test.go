package xraysql

import "database/sql/driver"

// compile time checking to satisfy the interface
// https://golang.org/doc/effective_go.html#blank_implements
var _ driver.Driver = (*driverDriver)(nil)
var _ driver.DriverContext = (*driverDriver)(nil)
