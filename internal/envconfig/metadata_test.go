package envconfig

import (
	"fmt"
	"testing"
	"time"
)

func TestMetadataServiceTimeout(t *testing.T) {
	tests := []struct {
		name string
		want time.Duration
	}{
		{
			name: "",
			want: time.Second,
		},
		{
			name: "1",
			want: time.Second,
		},
		{
			name: "invalid",
			want: time.Second,
		},
		{
			name: "10",
			want: 10 * time.Second,
		},
		{
			name: "0",
			want: time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.name), func(t *testing.T) {
			t.Setenv("AWS_METADATA_SERVICE_TIMEOUT", tt.name)
			if got := MetadataServiceTimeout(); got != tt.want {
				t.Errorf("MetadataServiceTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMetadataServiceNumAttempts(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{
			name: "",
			want: 1,
		},
		{
			name: "1",
			want: 1,
		},
		{
			name: "invalid",
			want: 1,
		},
		{
			name: "10",
			want: 10,
		},
		{
			name: "0",
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.name), func(t *testing.T) {
			t.Setenv("AWS_METADATA_SERVICE_NUM_ATTEMPTS", tt.name)
			if got := MetadataServiceNumAttempts(); got != tt.want {
				t.Errorf("MetadataServiceNumAttempts() = %v, want %v", got, tt.want)
			}
		})
	}
}
