// Package xraysql provides AWS X-Ray tracing for SQL.
//
// xraysql is drop-in replacement for [database/sql].
// Use [xraysql.Open] instead of [database/sql.Open].
//
//	db, err := xraysql.Open("postgres", "postgres://user:password@host:port/db")
//	if err != nil {
//		panic(err)
//	}
//
//	row, err := db.QueryRowContext(ctx, "SELECT 1")
package xraysql
