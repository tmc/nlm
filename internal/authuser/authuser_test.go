package authuser

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"empty", "", ""},
		{"zero", "0", ""},
		{"spaced zero", " 0 ", ""},
		{"explicit account", "2", "2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Normalize(test.value); got != test.want {
				t.Fatalf("Normalize(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}
