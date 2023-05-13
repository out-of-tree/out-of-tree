package debian

import (
	"github.com/rapidloop/skv"
)

type Cache struct {
	store *skv.KVStore
}

func NewCache(path string) (c *Cache, err error) {
	c = &Cache{}
	c.store, err = skv.Open(path)
	return
}

func (c Cache) Put(p DebianKernel) error {
	return c.store.Put(p.Version.Package, p)
}

func (c Cache) Get(version string) (p DebianKernel, err error) {
	err = c.store.Get(version, &p)
	return
}

func (c Cache) PutVersions(versions []string) error {
	return c.store.Put("versions", versions)
}

func (c Cache) GetVersions() (versions []string, err error) {
	err = c.store.Get("versions", &versions)
	return
}

func (c Cache) Close() error {
	return c.store.Close()
}
