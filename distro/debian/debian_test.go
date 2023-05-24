package debian

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"code.dumpstack.io/tools/out-of-tree/distro"
)

func TestDebian(t *testing.T) {
	assert := assert.New(t)

	u := Debian{release: Wheezy}

	assert.True(u.Equal(distro.Distro{Release: "wheezy", ID: distro.Debian}))

	assert.NotEmpty(u.Packages())
}

func TestKernelRelease(t *testing.T) {
	kernels, err := GetKernels()
	if err != nil {
		t.Fatal(err)
	}

	for _, k := range kernels {
		r, err := kernelRelease(k)
		if err != nil {
			t.Log(k.Version, r, err)
		}
	}

	for _, k := range kernels {
		r, err := kernelRelease(k)
		if err != nil {
			continue
		}

		if r == Wheezy {
			t.Log("Wheezy", k.Version)
		}

		if r == Jessie {
			t.Log("Jessie", k.Version)
		}
	}

}
