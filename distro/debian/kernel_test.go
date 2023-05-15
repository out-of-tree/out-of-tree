package debian

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestGetDebianKernel(t *testing.T) {
	dk, err := getDebianKernel("4.17.14-1")
	if err != nil {
		t.Fatal(err)
	}

	if dk.Version.ABI != "4.17.0-2" {
		t.Fatalf("wrong abi")
	}

	t.Logf("%s", spew.Sdump(dk))
}
