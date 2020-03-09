package xraysql

import (
	"context"
	"database/sql/driver"
)

type driverConnector struct {
	driver.Connector
	driver *driverDriver
}

func (c *driverConnector) Connect(ctx context.Context) (driver.Conn, error) {
	return nil, nil
}

func (c *driverConnector) Driver() driver.Driver {
	return c.driver
}

type fallbackConnector struct {
	driver driver.Driver
	name   string
}

func (c *fallbackConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.name)
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

func (d *driverDriver) OpenConnector(name string) (driver.Connector, error) {
	return nil, nil
}
