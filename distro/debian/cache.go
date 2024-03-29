package debian

import (
	"errors"
	"sync"

	"github.com/rapidloop/skv"
)

type Cache struct {
	store *skv.KVStore
}

// cache is not thread-safe, so make sure there are only one user
var mu sync.Mutex

func NewCache(path string) (c *Cache, err error) {
	mu.Lock()

	c = &Cache{}
	c.store, err = skv.Open(path)
	return
}

func (c Cache) Put(p []DebianKernel) error {
	if len(p) == 0 {
		return errors.New("empty slice")
	}
	return c.store.Put(p[0].Version.Package, p)
}

func (c Cache) Get(version string) (p []DebianKernel, err error) {
	err = c.store.Get(version, &p)
	if len(p) == 0 {
		err = skv.ErrNotFound
	}
	return
}

func (c Cache) PutVersions(versions []string) error {
	return c.store.Put("versions", versions)
}

func (c Cache) GetVersions() (versions []string, err error) {
	err = c.store.Get("versions", &versions)
	return
}

func (c Cache) Close() (err error) {
	err = c.store.Close()
	mu.Unlock()
	return
}
