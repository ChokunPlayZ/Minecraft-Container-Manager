package protocol

import "testing"

func TestSupportedVersionsOrder(t *testing.T) {
	for i := 1; i < len(SupportedVersions); i++ {
		if SupportedVersions[i-1].Protocol <= SupportedVersions[i].Protocol {
			t.Fatalf("SupportedVersions not in descending order at index %d: %d <= %d",
				i, SupportedVersions[i-1].Protocol, SupportedVersions[i].Protocol)
		}
	}
}

func TestLookup(t *testing.T) {
	tests := []struct {
		protocol int32
		name     string
		ok       bool
	}{
		{protocol: 776, name: "26.2", ok: true},
		{protocol: 775, name: "26.1", ok: true},
		{protocol: 774, name: "26.0", ok: true},
		{protocol: 773, name: "1.21.8", ok: true},
		{protocol: 762, name: "1.19.4", ok: true},
		{protocol: 999, ok: false},
	}

	for _, tt := range tests {
		v, err := Lookup(tt.protocol)
		if tt.ok {
			if err != nil {
				t.Fatalf("Lookup(%d) unexpected error: %v", tt.protocol, err)
			}
			if v.Protocol != tt.protocol || v.Name != tt.name {
				t.Fatalf("Lookup(%d) = %+v, want name %q", tt.protocol, v, tt.name)
			}
		} else if err == nil {
			t.Fatalf("Lookup(%d) expected error, got %+v", tt.protocol, v)
		}
	}
}

func TestLookup776Flags(t *testing.T) {
	v, err := Lookup(776)
	if err != nil {
		t.Fatalf("Lookup(776) unexpected error: %v", err)
	}
	if !v.HasConfigurationPhase {
		t.Error("776 should have configuration phase")
	}
	if !v.SupportsTransfer {
		t.Error("776 should support transfer")
	}
}
