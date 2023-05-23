package oraclelinux

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"code.dumpstack.io/tools/out-of-tree/distro"
)

func TestOracleLinux(t *testing.T) {
	assert := assert.New(t)

	u := OracleLinux{release: "9", container: "out_of_tree_oraclelinux_9"}

	assert.Equal(u.ID(), distro.OracleLinux)
	assert.Equal(u.Release(), "9")

	assert.True(u.Equal(distro.Distro{Release: "9", ID: distro.OracleLinux}))

	assert.NotEmpty(u.Packages())
}
