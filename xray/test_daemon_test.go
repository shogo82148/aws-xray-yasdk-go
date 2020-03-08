package xray

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

func TestTestDaemon(t *testing.T) {
	_, td := NewTestDaemon(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("hello"))
		if err != nil {
			panic(err)
		}
	}))
	defer td.Close()

	conn, err := dialer.DialContext(context.Background(), "udp", td.conn.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Write([]byte(`{"format":"json","version":1}
{
	"name" : "example.com",
	"id" : "70de5b6f19ff9a0a",
	"start_time" : 1.0E9,
	"trace_id" : "1-581cf771-a006649127e371903a2de979",
	"end_time" : 1.0E9
}`))
	if err != nil {
		t.Fatal(err)
	}

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name:      "example.com",
		ID:        "70de5b6f19ff9a0a",
		TraceID:   "1-581cf771-a006649127e371903a2de979",
		StartTime: 1e9,
		EndTime:   1e9,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
