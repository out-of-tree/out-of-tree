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
