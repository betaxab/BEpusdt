package utils

import "testing"

func TestIsValidTonAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    bool
	}{
		{
			name:    "non-bounceable address",
			address: "UQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAJKZ",
			want:    true,
		},
		{
			name:    "bounceable address is not accepted for payments",
			address: "EQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAM9c",
			want:    false,
		},
		{
			name:    "malformed address",
			address: "UQ-not-a-ton-address",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidTonAddress(tt.address); got != tt.want {
				t.Fatalf("IsValidTonAddress(%q) = %v, want %v", tt.address, got, tt.want)
			}
		})
	}
}
