package xraysql

import (
	"database/sql/driver"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// compile time checking to satisfy the interface
// https://golang.org/doc/effective_go.html#blank_implements
var _ driver.Conn = (*driverConn)(nil)
var _ driver.ConnBeginTx = (*driverConn)(nil)
var _ driver.ConnPrepareContext = (*driverConn)(nil)
var _ driver.NamedValueChecker = (*driverConn)(nil)
var _ driver.SessionResetter = (*driverConn)(nil)

func TestConn_Exec(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConn_Exec",
		ConnType: "fakeConnExt",
		Expect: []FakeExpect{
			&ExpectQuery{
				Query:   "SELECT version(), current_user, current_database()",
				Columns: []string{"version()", "current_user", "current_database()"},
				Rows: [][]driver.Value{
					{"0.0.0", "postgresql_user", "postgresql"},
				},
			},
			&ExpectExec{
				Query: "INSERT INTO products VALUES (?, ?, ?)",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	func() {
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		db := OpenDB(rawConnector)
		defer db.Close()
		_, err := db.ExecContext(ctx, "INSERT INTO products VALUES (?, ?, ?)", 1, "Cheese", 9.99)
		if err != nil {
			t.Fatal(err)
		}
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
			{
				Name:      "postgresql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
				Namespace: "remote",
				SQL: &schema.SQL{
					SanitizedQuery:  "INSERT INTO products VALUES (?, ?, ?)",
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

func TestConn_ExecContext(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConn_ExecContext",
		ConnType: "fakeConnCtx",
		Expect: []FakeExpect{
			&ExpectQuery{
				Query:   "SELECT version(), current_user, current_database()",
				Columns: []string{"version()", "current_user", "current_database()"},
				Rows: [][]driver.Value{
					{"0.0.0", "postgresql_user", "postgresql"},
				},
			},
			&ExpectExec{
				Query: "INSERT INTO products VALUES (?, ?, ?)",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	func() {
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		db := OpenDB(rawConnector)
		defer db.Close()
		_, err := db.ExecContext(ctx, "INSERT INTO products VALUES (?, ?, ?)", 1, "Cheese", 9.99)
		if err != nil {
			t.Fatal(err)
		}
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
			{
				Name:      "postgresql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
				Namespace: "remote",
				SQL: &schema.SQL{
					SanitizedQuery:  "INSERT INTO products VALUES (?, ?, ?)",
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

func TestConn_Query(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConn_Query",
		ConnType: "fakeConnExt",
		Expect: []FakeExpect{
			&ExpectQuery{
				Query:   "SELECT version(), current_user, current_database()",
				Columns: []string{"version()", "current_user", "current_database()"},
				Rows: [][]driver.Value{
					{"0.0.0", "postgresql_user", "postgresql"},
				},
			},
			&ExpectQuery{
				Query:   "SELECT id, name price FROM products WHERE id = ?",
				Columns: []string{"id", "name", "price"},
				Rows: [][]driver.Value{
					{1, "Cheese", 9.99},
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
		db := OpenDB(rawConnector)
		defer db.Close()
		row := db.QueryRowContext(ctx, "SELECT id, name price FROM products WHERE id = ?", 1)
		var (
			id    int64
			name  string
			price float64
		)
		if err := row.Scan(&id, &name, &price); err != nil {
			t.Fatal(err)
		}
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
			{
				Name:      "postgresql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
				Namespace: "remote",
				SQL: &schema.SQL{
					SanitizedQuery:  "SELECT id, name price FROM products WHERE id = ?",
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

func TestConn_QueryContext(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestConn_QueryContext",
		ConnType: "fakeConnCtx",
		Expect: []FakeExpect{
			&ExpectQuery{
				Query:   "SELECT version(), current_user, current_database()",
				Columns: []string{"version()", "current_user", "current_database()"},
				Rows: [][]driver.Value{
					{"0.0.0", "postgresql_user", "postgresql"},
				},
			},
			&ExpectQuery{
				Query:   "SELECT id, name price FROM products WHERE id = ?",
				Columns: []string{"id", "name", "price"},
				Rows: [][]driver.Value{
					{1, "Cheese", 9.99},
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
		db := OpenDB(rawConnector)
		row := db.QueryRowContext(ctx, "SELECT id, name price FROM products WHERE id = ?", 1)
		var (
			id    int64
			name  string
			price float64
		)
		if err := row.Scan(&id, &name, &price); err != nil {
			t.Fatal(err)
		}
		defer db.Close()
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
			{
				Name:      "postgresql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
				Namespace: "remote",
				SQL: &schema.SQL{
					SanitizedQuery:  "SELECT id, name price FROM products WHERE id = ?",
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
