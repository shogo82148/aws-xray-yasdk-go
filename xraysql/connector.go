package xraysql

import (
	"context"
	"database/sql/driver"
)

// NewConnector wraps the connector.
func NewConnector(c driver.Connector, opts ...Option) driver.Connector {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	d := &driverDriver{
		Driver: c.Driver(),
		config: cfg,
	}

	return &driverConnector{
		Connector: c,
		driver:    d,
	}
}

type driverConnector struct {
	driver.Connector
	driver *driverDriver
}

func (c *driverConnector) Connect(ctx context.Context) (driver.Conn, error) {
	rawConn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	conn := &driverConn{
		Conn: rawConn,
	}
	return conn, nil
}

func (c *driverConnector) Driver() driver.Driver {
	return c.driver
}

type fallbackConnector struct {
	driver         *driverDriver
	dataSourceName string
}

func (c *fallbackConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.driver.Driver.Open(c.dataSourceName)
	if err != nil {
		return nil, err
	}
	select {
	default:
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	}
	return conn, nil
}

func (c *fallbackConnector) Driver() driver.Driver {
	return c.driver
}

func (d *driverDriver) OpenConnector(dataSourceName string) (driver.Connector, error) {
	var c driver.Connector
	if dctx, ok := d.Driver.(driver.DriverContext); ok {
		var err error
		c, err = dctx.OpenConnector(dataSourceName)
		if err != nil {
			return nil, err
		}
	} else {
		c = &fallbackConnector{
			driver:         d,
			dataSourceName: dataSourceName,
		}
	}
	c = &driverConnector{
		Connector: c,
		driver:    d,
	}
	return c, nil
}
