package debian

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"

	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
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

	kernels, err := GetKernelsWithLimit(16, NoMode)
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

func TestKbuildVersion(t *testing.T) {
	assert := assert.New(t)

	kernels, err := GetKernelsWithLimit(16, NoMode)
	assert.Nil(err)
	assert.NotEmpty(kernels)

	toolsVersions, err := snapshot.SourcePackageVersions("linux-tools")
	assert.Nil(err)

	for _, dk := range kernels {
		if !kver(dk.Version.Package).LessThan(kver("4.5-rc0")) {
			continue
		}

		version := kbuildVersion(
			toolsVersions,
			dk.Version.Package,
		)
		assert.Nil(err)
		assert.NotEmpty(version)

		t.Log(dk.Version.Package, "->", version)
	}
}
