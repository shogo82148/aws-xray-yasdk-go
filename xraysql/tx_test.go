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
var _ driver.Tx = (*driverTx)(nil)

func TestTx_Commit(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestTx_Commit",
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

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		_, err = tx.ExecContext(ctx, "INSERT INTO products VALUES (?, ?, ?)", 1, "Cheese", 9.99)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
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
				Name: "transaction",
				Metadata: map[string]interface{}{
					"sql": map[string]interface{}{
						"tx_options": map[string]interface{}{
							"isolation_level": "Default",
							"read_only":       false,
						},
					},
				},
				Subsegments: []*schema.Segment{
					{
						Name:      "postgresql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
						Namespace: "remote",
						SQL: &schema.SQL{
							SanitizedQuery:  "BEGIN",
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
					{
						Name:      "postgresql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
						Namespace: "remote",
						SQL: &schema.SQL{
							SanitizedQuery:  "COMMIT",
							DriverVersion:   "github.com/shogo82148/aws-xray-yasdk-go/xraysql",
							DatabaseType:    "Postgres",
							DatabaseVersion: "0.0.0",
							User:            "postgresql_user",
						},
					},
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestTx_Rollback(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	rawConnector, err := fdriverctx.OpenConnectorWithOption(&FakeConnOption{
		Name:     "TestTx_Rollback",
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

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		_, err = tx.ExecContext(ctx, "INSERT INTO products VALUES (?, ?, ?)", 1, "Cheese", 9.99)
		if err != nil {
			t.Fatal(err)
		}
		// do not commit here for testing rollback
		// if err := tx.Commit(); err != nil {
		// 	t.Fatal(err)
		// }
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
				Name: "transaction",
				Metadata: map[string]interface{}{
					"sql": map[string]interface{}{
						"tx_options": map[string]interface{}{
							"isolation_level": "Default",
							"read_only":       false,
						},
					},
				},
				Fault: true,
				Subsegments: []*schema.Segment{
					{
						Name:      "postgresql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
						Namespace: "remote",
						SQL: &schema.SQL{
							SanitizedQuery:  "BEGIN",
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
					{
						Name:      "postgresql@github.com/shogo82148/aws-xray-yasdk-go/xraysql",
						Namespace: "remote",
						SQL: &schema.SQL{
							SanitizedQuery:  "ROLLBACK",
							DriverVersion:   "github.com/shogo82148/aws-xray-yasdk-go/xraysql",
							DatabaseType:    "Postgres",
							DatabaseVersion: "0.0.0",
							User:            "postgresql_user",
						},
					},
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
