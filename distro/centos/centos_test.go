package centos

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"code.dumpstack.io/tools/out-of-tree/distro"
)

func TestCentOS(t *testing.T) {
	assert := assert.New(t)

	u := CentOS{release: "7"}

	assert.True(u.Equal(distro.Distro{Release: "7", ID: distro.CentOS}))

	assert.NotEmpty(u.Packages())
}
