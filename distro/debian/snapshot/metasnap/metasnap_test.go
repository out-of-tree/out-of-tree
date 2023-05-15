package metasnap

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestGetPackageInfo(t *testing.T) {
	// existing
	infos, err := GetPackageInfo("linux-image-3.8-trunk-amd64",
		"amd64", "3.8.2-1~experimental.1")
	if err != nil {
		t.Fatal(err)
	}

	t.Log(spew.Sdump(infos))

	// non-existing
	infos, err = GetPackageInfo("meh", "amd64", "meh")
	if err == nil {
		t.Fatalf("should not be ok, result: %s", spew.Sdump(infos))
	}

	if err != ErrNotFound {
		t.Fatal("wrong error type")
	}
}
