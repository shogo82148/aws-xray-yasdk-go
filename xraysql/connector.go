package xraysql

import (
	"context"
	"database/sql/driver"
	"fmt"
	"io"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
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

	mu   sync.RWMutex
	attr *dbAttribute
}

func (c *driverConnector) Connect(ctx context.Context) (driver.Conn, error) {
	attr, err := c.getAttr(ctx)
	if err != nil {
		return nil, err
	}

	var rawConn driver.Conn
	err = xray.Capture(ctx, attr.name, func(ctx context.Context) error {
		attr.populate(ctx, "CONNECT")
		var err error
		rawConn, err = c.Connector.Connect(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	conn := &driverConn{
		Conn: rawConn,
		attr: attr,
	}
	return conn, nil
}

func (c *driverConnector) getAttr(ctx context.Context) (*dbAttribute, error) {
	c.mu.RLock()
	attr := c.attr
	c.mu.RUnlock()
	if attr != nil {
		return attr, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attr != nil {
		return c.attr, nil
	}

	err := xray.Capture(ctx, "detect database type", func(ctx context.Context) error {
		conn, err := c.Connector.Connect(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()

		attr, err = newDBAttribute(ctx, c.driver.baseName, c.driver.Driver, conn, c.driver.config)
		if err != nil {
			return err
		}
		c.attr = attr
		return nil
	})
	return c.attr, err
}

func (c *driverConnector) Driver() driver.Driver {
	return c.driver
}

type fallbackConnector struct {
	driver         driver.Driver
	dataSourceName string
}

func (c *fallbackConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.dataSourceName)
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
			driver:         d.Driver,
			dataSourceName: dataSourceName,
		}
	}
	c = &driverConnector{
		Connector: c,
		driver:    d,
	}
	return c, nil
}

type dbAttribute struct {
	connectionString string
	url              string
	databaseType     string
	databaseVersion  string
	driverVersion    string
	user             string
	name             string
	dbname           string
}

func newDBAttribute(ctx context.Context, driverName string, d driver.Driver, conn driver.Conn, cfg config) (*dbAttribute, error) {
	var attr dbAttribute

	// Detect database type and use that to populate attributes
	var detectors []func(ctx context.Context, conn driver.Conn, attr *dbAttribute) error
	switch driverName {
	case "postgres":
		detectors = append(detectors, postgresDetector)
	case "mysql":
		detectors = append(detectors, mysqlDetector)
	default:
		detectors = append(detectors, postgresDetector, mysqlDetector, mssqlDetector, oracleDetector)
	}
	for _, detector := range detectors {
		err := detector(ctx, conn, &attr)
		if err == nil {
			break
		}
	}

	attr.driverVersion = getDriverVersion(d)
	if driverName != "" {
		attr.name = attr.dbname + "@" + driverName
	} else {
		attr.name = attr.dbname + "@" + attr.driverVersion
	}

	if cfg.connectionString != "" {
		attr.connectionString = cfg.connectionString
	}
	if cfg.url != "" {
		attr.url = cfg.url
	}
	if cfg.name != "" {
		attr.name = cfg.name
	}
	return &attr, nil
}

func getDriverVersion(d driver.Driver) string {
	t := reflect.TypeOf(d)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	pkg := t.PkgPath()

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return pkg
	}

	version := ""
	depth := 0
	for _, dep := range info.Deps {
		// search the most specific module
		if strings.HasPrefix(pkg, dep.Path) && len(dep.Path) > depth {
			version = dep.Version
			depth = len(dep.Path)
		}
	}

	if version == "" {
		return pkg
	}
	return pkg + "@" + version
}

func postgresDetector(ctx context.Context, conn driver.Conn, attr *dbAttribute) error {
	var databaseVersion, user, dbname string
	err := queryRow(
		ctx, conn,
		"SELECT version(), current_user, current_database()",
		&databaseVersion, &user, &dbname,
	)
	if err != nil {
		return err
	}
	attr.databaseType = "Postgres"
	attr.databaseVersion = databaseVersion
	attr.user = user
	attr.dbname = dbname
	return nil
}

func mysqlDetector(ctx context.Context, conn driver.Conn, attr *dbAttribute) error {
	var databaseVersion, user, dbname string
	err := queryRow(
		ctx, conn,
		"SELECT version(), current_user(), database()",
		&databaseVersion, &user, &dbname,
	)
	if err != nil {
		return err
	}
	attr.databaseType = "MySQL"
	attr.databaseVersion = databaseVersion
	attr.user = user
	attr.dbname = dbname
	return nil
}

func mssqlDetector(ctx context.Context, conn driver.Conn, attr *dbAttribute) error {
	var databaseVersion, user, dbname string
	err := queryRow(
		ctx, conn,
		"SELECT @@version, current_user, db_name()",
		&databaseVersion, &user, &dbname,
	)
	if err != nil {
		return err
	}
	attr.databaseType = "MS SQL"
	attr.databaseVersion = databaseVersion
	attr.user = user
	attr.dbname = dbname
	return nil
}

func oracleDetector(ctx context.Context, conn driver.Conn, attr *dbAttribute) error {
	var databaseVersion, user, dbname string
	err := queryRow(
		ctx, conn,
		"SELECT version FROM v$instance UNION SELECT user, ora_database_name FROM dual",
		&databaseVersion, &user, &dbname,
	)
	if err != nil {
		return err
	}
	attr.databaseType = "Oracle"
	attr.databaseVersion = databaseVersion
	attr.user = user
	attr.dbname = dbname
	return nil
}

// minimum implementation of (*sql.DB).QueryRow
func queryRow(ctx context.Context, conn driver.Conn, query string, dest ...*string) error {
	var err error

	// prepare
	var stmt driver.Stmt
	if connCtx, ok := conn.(driver.ConnPrepareContext); ok {
		stmt, err = connCtx.PrepareContext(ctx, query)
	} else {
		stmt, err = conn.Prepare(query)
		if err == nil {
			select {
			default:
			case <-ctx.Done():
				stmt.Close()
				return ctx.Err()
			}
		}
	}
	if err != nil {
		return err
	}
	defer stmt.Close()

	// execute query
	var rows driver.Rows
	if queryCtx, ok := stmt.(driver.StmtQueryContext); ok {
		rows, err = queryCtx.QueryContext(ctx, []driver.NamedValue{})
	} else {
		select {
		default:
		case <-ctx.Done():
			return ctx.Err()
		}
		rows, err = stmt.Query([]driver.Value{})
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	// scan
	if len(dest) != len(rows.Columns()) {
		return fmt.Errorf("xraysql: expected %d destination arguments in Scan, not %d", len(dest), len(rows.Columns()))
	}
	cols := make([]driver.Value, len(rows.Columns()))
	if err := rows.Next(cols); err != nil {
		return err
	}
	for i, src := range cols {
		d := dest[i]
		switch s := src.(type) {
		case string:
			*d = s
		case []byte:
			*d = string(s)
		case time.Time:
			*d = s.Format(time.RFC3339Nano)
		case int64:
			*d = strconv.FormatInt(s, 10)
		case float64:
			*d = strconv.FormatFloat(s, 'g', -1, 64)
		case bool:
			*d = strconv.FormatBool(s)
		default:
			return fmt.Errorf("sql: Scan error on column index %d, name %q: type missmatch", i, rows.Columns()[i])
		}
	}

	return nil
}

func (attr *dbAttribute) populate(ctx context.Context, query string) {
	seg := xray.ContextSegment(ctx)
	sqlData := &schema.SQL{
		ConnectionString: attr.connectionString,
		URL:              attr.url,
		DatabaseType:     attr.databaseType,
		DatabaseVersion:  attr.databaseVersion,
		DriverVersion:    attr.driverVersion,
		User:             attr.user,
		SanitizedQuery:   query,
	}
	seg.SetSQL(sqlData)
	seg.SetNamespace("remote")
}

// from Go 1.17, the DB.Close method closes the connector field.
func (c *driverConnector) Close() error {
	if c, ok := c.Connector.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
