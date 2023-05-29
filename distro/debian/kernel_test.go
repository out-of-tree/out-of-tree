package debian

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

func TestGetDebianKernel(t *testing.T) {
	assert := assert.New(t)

	dk, err := getDebianKernel("4.6.4-1")
	assert.Nil(err)

	assert.Equal(getRelease(dk.Image), Stretch)

	t.Logf("%s", spew.Sdump(dk))
}

func TestParseKernelVersion(t *testing.T) {
	assert := assert.New(t)

	kernels, err := GetKernels()
	assert.Nil(err)
	assert.NotEmpty(kernels)

	versions := make(map[string]bool)

	for _, dk := range kernels {
		dkv, err := ParseKernelVersion(dk.Image.Deb.Name)
		assert.Nil(err)

		_, found := versions[dkv.Package]
		assert.True(!found)

		versions[dkv.Package] = true
	}
}
