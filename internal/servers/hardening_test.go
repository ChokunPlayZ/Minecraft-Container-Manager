package servers

import "testing"

func TestValidateLimits(t *testing.T) {
	cases := []struct {
		name     string
		cpu      float64
		memoryMB int
		wantErr  bool
	}{
		{"defaults", 0, 0, false},
		{"cpu", 2.0, 0, false},
		{"memory", 0, 4096, false},
		{"both", 1.5, 2048, false},
		{"negative cpu", -0.5, 0, true},
		{"negative memory", 0, -1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLimits(tc.cpu, tc.memoryMB)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for cpu=%v memoryMB=%d", tc.cpu, tc.memoryMB)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
