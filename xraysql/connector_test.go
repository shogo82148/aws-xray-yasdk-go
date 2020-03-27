package xraysql

import (
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// compile time checking to satisfy the interface
// https://golang.org/doc/effective_go.html#blank_implements
var _ driver.Connector = (*driverConnector)(nil)
var _ driver.Connector = (*fallbackConnector)(nil)

func TestConnect_postgresql(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConnect",
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
		Name: "test",
		Subsegments: []*schema.Segment{
			{Name: "detect database type"},
			{
				Name:      "postgresql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
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
		AWS: &schema.AWS{
			XRay: &schema.XRay{
				Version: xray.Version,
				Type:    xray.Type,
			},
		},
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestConnect_mysql(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConnect",
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
		Name: "test",
		Subsegments: []*schema.Segment{
			{Name: "detect database type"},
			{
				Name:      "mysql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
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
		AWS: &schema.AWS{
			XRay: &schema.XRay{
				Version: xray.Version,
				Type:    xray.Type,
			},
		},
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestConnect_mssql(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConnect",
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
		Name: "test",
		Subsegments: []*schema.Segment{
			{Name: "detect database type"},
			{
				Name:      "mssql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
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
		AWS: &schema.AWS{
			XRay: &schema.XRay{
				Version: xray.Version,
				Type:    xray.Type,
			},
		},
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestConnect_oracle(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConnect",
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
		Name: "test",
		Subsegments: []*schema.Segment{
			{Name: "detect database type"},
			{
				Name:      "oracle@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
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
		AWS: &schema.AWS{
			XRay: &schema.XRay{
				Version: xray.Version,
				Type:    xray.Type,
			},
		},
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
