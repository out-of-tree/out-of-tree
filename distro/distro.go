package distro

import (
	"errors"
	"sync"
)

var mu sync.Mutex
var distros []distribution

type distribution interface {
	Distro() Distro
	Equal(Distro) bool
	Packages() (packages []string, err error)
	Install(pkg string, headers bool) (err error)
	Kernels() (kernels []KernelInfo, err error)
}

func Register(d distribution) {
	mu.Lock()
	defer mu.Unlock()

	distros = append(distros, d)
}

func List() (dds []Distro) {
	for _, dd := range distros {
		dds = append(dds, dd.Distro())
	}
	return
}

type Distro struct {
	ID      ID
	Release string
}

func (d Distro) Packages() (packages []string, err error) {
	for _, dd := range distros {
		if d.ID != None && d.ID != dd.Distro().ID {
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

func (d Distro) Install(pkg string, headers bool) (err error) {
	for _, dd := range distros {
		if !dd.Equal(d) {
			continue
		}

		return dd.Install(pkg, headers)
	}
	return errors.New("not found")
}

func (d Distro) Kernels() (kernels []KernelInfo, err error) {
	for _, dd := range distros {
		if dd.Equal(d) {
			return dd.Kernels()
		}
	}
	return
}
