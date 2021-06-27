package xraysql

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// compile time checking to satisfy the interface
// https://golang.org/doc/effective_go.html#blank_implements
var _ driver.Connector = (*driverConnector)(nil)
var _ io.Closer = (*driverConnector)(nil)
var _ driver.Connector = (*fallbackConnector)(nil)

func TestConnect_postgresql(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConnect_postgresql",
		ConnType: "fakeConn",
		Expect: []FakeExpect{
			&ExpectQuery{
				Query:   "SELECT version(), current_user, current_database()",
				Columns: []string{"version()", "current_user", "current_database()"},
				Rows: [][]driver.Value{
					{"0.0.0", "postgresql_user", "postgresql"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	func() {
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		connector := NewConnector(rawConnector)
		conn, err := connector.Connect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
	}()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "detect database type",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
			},
			{
				Name:      "postgresql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				SQL: &schema.SQL{
					SanitizedQuery:  "CONNECT",
					DriverVersion:   "github.com/shogo82148/aws-xray-yasdk-go/xraysql",
					DatabaseType:    "Postgres",
					DatabaseVersion: "0.0.0",
					User:            "postgresql_user",
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestConnect_mysql(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConnect_mysql",
		ConnType: "fakeConn",
		Expect: []FakeExpect{
			&ExpectQuery{
				Query: "SELECT version(), current_user, current_database()",
				Err:   errors.New("not postgresql"),
			},
			&ExpectQuery{
				Query:   "SELECT version(), current_user(), database()",
				Columns: []string{"version()", "current_user", "database()"},
				Rows: [][]driver.Value{
					{"0.0.0", "mysql_user", "mysql"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	func() {
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		connector := NewConnector(rawConnector)
		conn, err := connector.Connect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
	}()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name:      "test",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		ID:        "xxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "detect database type",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
			},
			{
				Name:      "mysql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				SQL: &schema.SQL{
					SanitizedQuery:  "CONNECT",
					DriverVersion:   "github.com/shogo82148/aws-xray-yasdk-go/xraysql",
					DatabaseType:    "MySQL",
					DatabaseVersion: "0.0.0",
					User:            "mysql_user",
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestConnect_mssql(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConnect_mssql",
		ConnType: "fakeConn",
		Expect: []FakeExpect{
			&ExpectQuery{
				Query: "SELECT version(), current_user, current_database()",
				Err:   errors.New("not postgresql"),
			},
			&ExpectQuery{
				Query: "SELECT version(), current_user(), database()",
				Err:   errors.New("not mysql"),
			},
			&ExpectQuery{
				Query:   "SELECT @@version, current_user, db_name()",
				Columns: []string{"@@version", "current_user", "db_name()"},
				Rows: [][]driver.Value{
					{"0.0.0", "mssql_user", "mssql"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	func() {
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		connector := NewConnector(rawConnector)
		conn, err := connector.Connect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
	}()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "detect database type",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
			},
			{
				Name:      "mssql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				SQL: &schema.SQL{
					SanitizedQuery:  "CONNECT",
					DriverVersion:   "github.com/shogo82148/aws-xray-yasdk-go/xraysql",
					DatabaseType:    "MS SQL",
					DatabaseVersion: "0.0.0",
					User:            "mssql_user",
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestConnect_oracle(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConnect_oracle",
		ConnType: "fakeConn",
		Expect: []FakeExpect{
			&ExpectQuery{
				Query: "SELECT version(), current_user, current_database()",
				Err:   errors.New("not postgresql"),
			},
			&ExpectQuery{
				Query: "SELECT version(), current_user(), database()",
				Err:   errors.New("not mysql"),
			},
			&ExpectQuery{
				Query: "SELECT @@version, current_user, db_name()",
				Err:   errors.New("not mssql"),
			},
			&ExpectQuery{
				Query:   "SELECT version FROM v$instance UNION SELECT user, ora_database_name FROM dual",
				Columns: []string{"version", "user", "ora_database_name"},
				Rows: [][]driver.Value{
					{"0.0.0", "oracle_user", "oracle"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	func() {
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		connector := NewConnector(rawConnector)
		conn, err := connector.Connect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
	}()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "detect database type",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
			},
			{
				Name:      "oracle@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				SQL: &schema.SQL{
					SanitizedQuery:  "CONNECT",
					DriverVersion:   "github.com/shogo82148/aws-xray-yasdk-go/xraysql",
					DatabaseType:    "Oracle",
					DatabaseVersion: "0.0.0",
					User:            "oracle_user",
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestConnect_ConnContext(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConnect_ConnContext",
		ConnType: "fakeConnCtx",
		Expect: []FakeExpect{
			&ExpectQuery{
				Query:   "SELECT version(), current_user, current_database()",
				Columns: []string{"version()", "current_user", "current_database()"},
				Rows: [][]driver.Value{
					{"0.0.0", "postgresql_user", "postgresql"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	func() {
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		connector := NewConnector(rawConnector)
		conn, err := connector.Connect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
	}()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "detect database type",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
			},
			{
				Name:      "postgresql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				SQL: &schema.SQL{
					SanitizedQuery:  "CONNECT",
					DriverVersion:   "github.com/shogo82148/aws-xray-yasdk-go/xraysql",
					DatabaseType:    "Postgres",
					DatabaseVersion: "0.0.0",
					User:            "postgresql_user",
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

type closerConnector struct {
	// the result of Close() method
	errClose error

	// a flag whether Close() method is called
	closed bool
}

func (c *closerConnector) Connect(ctx context.Context) (driver.Conn, error) {
	panic("never used")
}

func (c *closerConnector) Driver() driver.Driver {
	return fdriverctx
}

func (c *closerConnector) Close() error {
	c.closed = true
	return c.errClose
}

func TestConnectorClose(t *testing.T) {
	t.Run("c.Connector doesn't implement io.Closer", func(t *testing.T) {
		c0 := &fakeConnector{
			driver: fdriverctx,
		}
		c1 := NewConnector(c0)
		if err := c1.(io.Closer).Close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Closing c.Connector succeeds", func(t *testing.T) {
		c0 := &closerConnector{}
		c1 := NewConnector(c0)
		if err := c1.(io.Closer).Close(); err != nil {
			t.Fatal(err)
		}
		if !c0.closed {
			t.Errorf("c.Connector should be closed, but not")
		}
	})

	t.Run("Closing c.Connector fails", func(t *testing.T) {
		errClose := errors.New("some error while closing")
		c0 := &closerConnector{
			errClose: errClose,
		}
		c1 := NewConnector(c0)
		if err := c1.(io.Closer).Close(); err != errClose {
			t.Errorf("want err is %v, got %v", errClose, err)
		}
		if !c0.closed {
			t.Errorf("c.Connector should be closed, but not")
		}
	})
}
