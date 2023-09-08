package xraysql

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
)

type config struct {
	connectionString string
	url              string
	name             string
}

// Option is an option of SQL tracer.
type Option func(*config)

// WithConnectionString configures the data source name that is recorded in the X-Ray segment.
// By default, the tracer doesn't record the data source to avoid recording secrets such as passwords.
func WithConnectionString(str string) Option {
	return func(cfg *config) {
		cfg.connectionString = str
	}
}

// WithURL configures the url of data source that is recorded in the X-Ray segment.
// By default, the tracer doesn't record the data source to avoid recording secrets such as passwords.
func WithURL(url string) Option {
	return func(cfg *config) {
		cfg.url = url
	}
}

// WithName configures the segment's name.
// By default, database_name@driver_name is used.
func WithName(name string) Option {
	return func(cfg *config) {
		cfg.name = name
	}
}

// Open is a drop-in replacement for [database/sql.Open].
// It returns a [*database/sql.DB] that is instrumented for AWS X-Ray.
func Open(driverName, dataSourceName string, opts ...Option) (*sql.DB, error) {
	name, err := registerDriver(driverName, dataSourceName, opts...)
	if err != nil {
		return nil, err
	}
	return sql.Open(name, dataSourceName)
}

func registerDriver(driverName, dataSourceName string, opts ...Option) (string, error) {
	// get original driver
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return "", err
	}
	defer db.Close()

	// wrap the driver
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	d := &driverDriver{
		Driver:   db.Driver(),
		config:   cfg,
		baseName: driverName,
	}

	// register the driver with the individual name
	name := fmt.Sprintf("xray-%p", d)
	sql.Register(name, d)
	return name, nil
}

// OpenDB is a drop-in replacement for [database/sql.OpenDB].
// It returns a [*database/sql.DB] that is instrumented for AWS X-Ray.
func OpenDB(c driver.Connector, opts ...Option) *sql.DB {
	return sql.OpenDB(NewConnector(c, opts...))
}

type driverDriver struct {
	driver.Driver
	config   config
	baseName string
}
