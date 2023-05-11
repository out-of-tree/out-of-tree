package debian

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rapidloop/skv"
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

	version := "4.17.14-1"

	dk, err := GetDebianKernel(version)
	if err != nil {
		t.Fatal(err)
	}

	err = c.Put(dk)
	if err != nil {
		t.Fatal(err)
	}

	dk2, err := c.Get(version)
	if err != nil {
		t.Fatal(err)
	}

	if dk.Image.Deb.Hash != dk2.Image.Deb.Hash {
		t.Fatalf("mismatch")
	}

	c.Close()

	c, err = NewCache(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	dk3, err := c.Get(version)
	if err != nil {
		t.Fatal(err)
	}

	if dk.Image.Deb.Hash != dk3.Image.Deb.Hash {
		t.Fatalf("mismatch")
	}

	_, err = c.Get("key not exist")
	if err == nil || err != skv.ErrNotFound {
		t.Fatal(err)
	}
}
