package ubuntu

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"code.dumpstack.io/tools/out-of-tree/distro"
)

func TestUbuntu(t *testing.T) {
	assert := assert.New(t)

	u := Ubuntu{release: "22.04"}

	assert.True(u.Equal(distro.Distro{Release: "22.04", ID: distro.Ubuntu}))

	assert.NotEmpty(u.Packages())
}
