package debian

import (
	"testing"

	"code.dumpstack.io/tools/out-of-tree/config"
)

func TestMatchImagePkg(t *testing.T) {
	km := config.KernelMask{ReleaseMask: "3.2.0-4"}

	pkgs, err := MatchImagePkg(km)
	if err != nil {
		t.Fatal(err)
	}

	if len(pkgs) == 0 {
		t.Fatal("no packages")
	}
}
