package docker

import "testing"

func TestContainerResources(t *testing.T) {
	mb := 1024 * 1024

	// No explicit limits: memory defaults to RAM+512MB, no CPU quota.
	res := containerResources(CreateOpts{RAMMB: 2048})
	if res.Memory != int64(2560)*int64(mb) {
		t.Errorf("default memory = %d, want %d", res.Memory, int64(2560)*int64(mb))
	}
	if res.NanoCPUs != 0 {
		t.Errorf("default NanoCPUs = %d, want 0", res.NanoCPUs)
	}

	// Explicit memory and CPU limits override defaults.
	res = containerResources(CreateOpts{RAMMB: 2048, CPULimit: 1.5, MemoryLimitMB: 4096})
	if res.Memory != int64(4096)*int64(mb) {
		t.Errorf("memory = %d, want %d", res.Memory, int64(4096)*int64(mb))
	}
	if res.NanoCPUs != int64(1.5*1e9) {
		t.Errorf("NanoCPUs = %d, want %d", res.NanoCPUs, int64(1.5*1e9))
	}
}
