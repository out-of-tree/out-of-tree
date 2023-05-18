package distro

import (
	"sync"
)

var mu sync.Mutex
var distros []distribution

type distribution interface {
	ID() ID
	Release() string
	Equal(Distro) bool
	Packages() (packages []string, err error)
}

func Register(d distribution) {
	mu.Lock()
	defer mu.Unlock()

	distros = append(distros, d)
}

func List() (dds []Distro) {
	mu.Lock()
	defer mu.Unlock()

	for _, dd := range distros {
		dds = append(dds, Distro{
			ID:      dd.ID(),
			Release: dd.Release(),
		})
	}

	return
}

type Distro struct {
	ID      ID
	Release string
}

func (d Distro) Packages() (packages []string, err error) {
	for _, dd := range distros {
		if d.ID != None && d.ID != dd.ID() {
			continue
		}

		if d.Release != "" && !dd.Equal(d) {
			continue
		}

		var pkgs []string
		pkgs, err = dd.Packages()
		if err != nil {
			return
		}

		packages = append(packages, pkgs...)
	}
	return
}
