package xraysql

import (
	"database/sql/driver"
	"strings"
	"testing"
	"unicode"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// compile time checking to satisfy the interface
// https://golang.org/doc/effective_go.html#blank_implements
var _ driver.Driver = (*driverDriver)(nil)
var _ driver.DriverContext = (*driverDriver)(nil)

// we check the format of strings, ignore their values.
func ignore(s string) string {
	var builder strings.Builder
	builder.Grow(len(s))
	for _, ch := range s {
		if unicode.IsLetter(ch) || unicode.IsNumber(ch) {
			builder.WriteRune('x')
		} else {
			builder.WriteRune(ch)
		}
	}
	return builder.String()
}

const timeFilled = 1234567890

// we check whether time is set
func ignoreTime(t float64) float64 {
	if t == 0 {
		return 0
	}
	return timeFilled
}

func ignoreVariableFieldFunc(in *schema.Segment) *schema.Segment {
	out := *in
	out.ID = ignore(out.ID)
	out.TraceID = ignore(out.TraceID)
	out.ParentID = ignore(out.ParentID)
	out.StartTime = ignoreTime(out.StartTime)
	out.EndTime = ignoreTime(out.EndTime)
	out.Subsegments = nil
	if out.AWS != nil {
		delete(out.AWS, "xray")
		if v, ok := out.AWS["request_id"].(string); ok {
			out.AWS["request_id"] = ignore(v)
		}
		if len(out.AWS) == 0 {
			out.AWS = nil
		}
	}
	if out.Cause != nil {
		for i := range out.Cause.Exceptions {
			out.Cause.Exceptions[i].ID = ignore(out.Cause.Exceptions[i].ID)
		}
	}
	for _, sub := range in.Subsegments {
		out.Subsegments = append(out.Subsegments, ignoreVariableFieldFunc(sub))
	}
	return &out
}

// some fields change every execution, ignore them.
var ignoreVariableField = cmp.Transformer("Segment", ignoreVariableFieldFunc)

func TestOpen_withFallbackConnector(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	dsn := AddOption(&FakeConnOption{
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

	func() {
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		db, err := Open("fakedb", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := db.PingContext(ctx); err != nil {
			t.Error(err)
		}
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
				Name:      "postgresql@fakedb",
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
			{
				Name:      "postgresql@fakedb",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				SQL: &schema.SQL{
					SanitizedQuery:  "PING",
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

func TestOpen(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	dsn := AddOption(&FakeConnOption{
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

	func() {
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		db, err := Open("fakedbctx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := db.PingContext(ctx); err != nil {
			t.Error(err)
		}
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
				Name:      "postgresql@fakedbctx",
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
			{
				Name:      "postgresql@fakedbctx",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				SQL: &schema.SQL{
					SanitizedQuery:  "PING",
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
