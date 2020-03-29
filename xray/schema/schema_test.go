package schema

import "testing"

func TestToSnakeCase(t *testing.T) {
	tc := []struct {
		in   string
		want string
	}{
		{in: "ParameterName", want: "parameter_name"},
		{in: "parameter_name", want: "parameter_name"},
		{in: "parameterName", want: "parameter_name"},
	}

	for _, tt := range tc {
		got := toSnakeCase(tt.in)
		if got != tt.want {
			t.Errorf("%q: want %q, got %q", tt.in, tt.want, got)
		}
	}
}
