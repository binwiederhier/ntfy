package sprig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOsBase(t *testing.T) {
	assert.NoError(t, runt(`{{ osBase "foo/bar" }}`, "bar"))
}

func TestOsDir(t *testing.T) {
	assert.NoError(t, runt(`{{ osDir "foo/bar/baz" }}`, "foo/bar"))
}

func TestOsIsAbs(t *testing.T) {
	assert.NoError(t, runt(`{{ osIsAbs "/foo" }}`, "true"))
	assert.NoError(t, runt(`{{ osIsAbs "foo" }}`, "false"))
}

func TestOsClean(t *testing.T) {
	assert.NoError(t, runt(`{{ osClean "/foo/../foo/../bar" }}`, "/bar"))
}

func TestOsExt(t *testing.T) {
	assert.NoError(t, runt(`{{ osExt "/foo/bar/baz.txt" }}`, ".txt"))
}
