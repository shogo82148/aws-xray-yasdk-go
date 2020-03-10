package xray

import (
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseTraceHeader(t *testing.T) {
	tc := []struct {
		s    string
		want TraceHeader
	}{
		{
			s:    "",
			want: TraceHeader{},
		},
		{
			s: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5",
			want: TraceHeader{
				TraceID: "1-5e645f3e-1dfad076a177c5ccc5de12f5",
			},
		},
		{
			s: "ROOT=1-5E645F3E-1DFAD076A177C5CCC5DE12F5",
			want: TraceHeader{
				TraceID: "1-5e645f3e-1dfad076a177c5ccc5de12f5",
			},
		},
		{
			s: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Parent=03babb4ba280be51",
			want: TraceHeader{
				TraceID:  "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				ParentID: "03babb4ba280be51",
			},
		},
		{
			s: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=1",
			want: TraceHeader{
				TraceID:          "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				SamplingDecision: SamplingDecisionSampled,
			},
		},
		{
			s: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=0",
			want: TraceHeader{
				TraceID:          "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				SamplingDecision: SamplingDecisionNotSampled,
			},
		},
		{
			s: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=?",
			want: TraceHeader{
				TraceID:          "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				SamplingDecision: SamplingDecisionRequested,
			},
		},
		{
			s: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5",
			want: TraceHeader{
				TraceID:          "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				SamplingDecision: SamplingDecisionUnknown,
			},
		},
		{
			s: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;foo=bar;hoge=fuga",
			want: TraceHeader{
				TraceID: "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				AdditionalData: map[string]string{
					"foo":  "bar",
					"hoge": "fuga",
				},
			},
		},
		{
			s: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;foo",
			want: TraceHeader{
				TraceID: "1-5e645f3e-1dfad076a177c5ccc5de12f5",
			},
		},
		{
			s: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=invalid",
			want: TraceHeader{
				TraceID: "1-5e645f3e-1dfad076a177c5ccc5de12f5",
			},
		},
		{
			s: "foo=bar;hoge=fuga;Sampled=1;Parent=03babb4ba280be51;Root=1-5e645f3e-1dfad076a177c5ccc5de12f5",
			want: TraceHeader{
				TraceID:          "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				ParentID:         "03babb4ba280be51",
				SamplingDecision: SamplingDecisionSampled,
				AdditionalData: map[string]string{
					"foo":  "bar",
					"hoge": "fuga",
				},
			},
		},
	}
	for i, tt := range tc {
		got := ParseTraceHeader(tt.s)
		if diff := cmp.Diff(tt.want, got); diff != "" {
			t.Errorf("%d: mismatch (-want +got):\n%s", i, diff)
		}
	}
}

func BenchmarkParseTraceHeader(b *testing.B) {
	s := "ROOT=1-5E645F3E-1DFAD076A177C5CCC5DE12F5;PARENT=03BABB4BA280BE51;SAMPLED=1;FOO=BAR;HOGE=FUGA"
	for i := 0; i < b.N; i++ {
		runtime.KeepAlive(ParseTraceHeader(s))
	}
}

func TestTraceHeader_String(t *testing.T) {
	tc := []struct {
		h    TraceHeader
		want string
	}{
		{
			h:    TraceHeader{},
			want: "",
		},
		{
			h: TraceHeader{
				TraceID: "1-5e645f3e-1dfad076a177c5ccc5de12f5",
			},
			want: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5",
		},
		{
			h: TraceHeader{
				TraceID:  "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				ParentID: "03babb4ba280be51",
			},
			want: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Parent=03babb4ba280be51",
		},
		{
			h: TraceHeader{
				TraceID:          "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				SamplingDecision: SamplingDecisionSampled,
			},
			want: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=1",
		},
		{
			h: TraceHeader{
				TraceID:          "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				SamplingDecision: SamplingDecisionNotSampled,
			},
			want: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=0",
		},
		{
			h: TraceHeader{
				TraceID:          "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				SamplingDecision: SamplingDecisionRequested,
			},
			want: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=?",
		},
		{
			h: TraceHeader{
				TraceID:          "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				SamplingDecision: SamplingDecisionUnknown,
			},
			want: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5",
		},
		{
			h: TraceHeader{
				TraceID: "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				AdditionalData: map[string]string{
					"foo":  "bar",
					"hoge": "fuga",
				},
			},
			want: "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;foo=bar;hoge=fuga",
		},
	}

	for i, tt := range tc {
		got := tt.h.String()
		if got != tt.want {
			t.Errorf("%d: want %s, got %s", i, tt.want, got)
		}
	}
}

func BenchmarkTraceHeader_String(b *testing.B) {
	h := TraceHeader{
		TraceID:          "1-5e645f3e-1dfad076a177c5ccc5de12f5",
		ParentID:         "03babb4ba280be51",
		SamplingDecision: SamplingDecisionSampled,
		AdditionalData: map[string]string{
			"foo":  "bar",
			"hoge": "fuga",
		},
	}
	for i := 0; i < b.N; i++ {
		runtime.KeepAlive(h.String())
	}
}
