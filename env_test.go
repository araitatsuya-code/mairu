package main

import "testing"

func TestNormalizeEnvValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: "value", want: "value"},
		{name: "trimmed", input: "  value  ", want: "value"},
		{name: "double quoted", input: `"value"`, want: "value"},
		{name: "single quoted", input: `'value'`, want: "value"},
		{name: "spaces in quotes", input: ` "value with spaces" `, want: "value with spaces"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeEnvValue(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeEnvValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
