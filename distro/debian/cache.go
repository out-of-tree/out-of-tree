package debian

import (
	"github.com/rapidloop/skv"

	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
)

type Cache struct {
	store *skv.KVStore
}

func NewCache(path string) (c *Cache, err error) {
	c = &Cache{}
	c.store, err = skv.Open(path)
	return
}

func (c Cache) Put(p snapshot.Package) error {
	return c.store.Put(p.Version, p)
}

func (c Cache) Get(version string) (p snapshot.Package, err error) {
	err = c.store.Get(version, &p)
	return
}

func (c Cache) Close() error {
	return c.store.Close()
}
