package debian

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rapidloop/skv"

	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
)

func TestCache(t *testing.T) {
	dir, err := os.MkdirTemp("", "out-of-tree_cache_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "debian.cache")

	c, err := NewCache(path)
	if err != nil {
		t.Fatal(err)
	}

	packages, err := snapshot.Packages("linux", "4.17.14-1", "amd64",
		`^linux-(image|headers)-[0-9\.\-]*-(amd64|amd64-unsigned)$`)
	if err != nil {
		t.Fatal(err)
	}

	err = c.Put(packages[0])
	if err != nil {
		t.Fatal(err)
	}

	p, err := c.Get(packages[0].Version)
	if err != nil {
		t.Fatal(err)
	}

	if p.Deb.Hash != packages[0].Deb.Hash {
		t.Fatalf("mismatch")
	}

	c.Close()

	c, err = NewCache(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	p, err = c.Get(packages[0].Version)
	if err != nil {
		t.Fatal(err)
	}

	if p.Deb.Hash != packages[0].Deb.Hash {
		t.Fatalf("mismatch")
	}

	p, err = c.Get("key not exist")
	if err == nil || err != skv.ErrNotFound {
		t.Fatal(err)
	}
}
