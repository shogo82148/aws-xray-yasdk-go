package xraysql

import "database/sql/driver"

// compile time checking to satisfy the interface
// https://golang.org/doc/effective_go.html#blank_implements
var _ driver.Stmt = (*driverStmt)(nil)
var _ driver.ColumnConverter = (*driverStmt)(nil)
var _ driver.StmtExecContext = (*driverStmt)(nil)
var _ driver.StmtQueryContext = (*driverStmt)(nil)
var _ driver.NamedValueChecker = (*driverStmt)(nil)
