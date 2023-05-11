package debian

import (
	"testing"
)

func TestGetDebianKernel(t *testing.T) {
	dk, err := GetDebianKernel("4.17.14-1")
	if err != nil {
		t.Fatal(err)
	}

	if dk.Version.ABI != "4.17.0-2" {
		t.Fatalf("wrong abi")
	}
}
