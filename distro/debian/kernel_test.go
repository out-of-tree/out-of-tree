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
