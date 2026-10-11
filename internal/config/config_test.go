package config

import (
	"runtime"
	"testing"
)

func TestDefaultWorkersMatchCPUs(t *testing.T) {
	c := Default()
	if c.Workers != runtime.NumCPU() {
		t.Fatalf("Workers = %d, want %d", c.Workers, runtime.NumCPU())
	}
}

func TestValidateCorrectsInvalidValues(t *testing.T) {
	c := Config{Workers: 0, BatchSize: -5, MaxFileSizeBytes: 0, ImpactDepth: 0}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Workers != 1 || c.BatchSize != 1 || c.ImpactDepth != 1 {
		t.Fatalf("unexpected values after Validate: %+v", c)
	}
	if c.MaxFileSizeBytes != Default().MaxFileSizeBytes {
		t.Fatalf("MaxFileSizeBytes not defaulted: %d", c.MaxFileSizeBytes)
	}
}
