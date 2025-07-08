package sprig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOsBase(t *testing.T) {
	assert.NoError(t, runt(`{{ osBase "C:\\foo\\bar" }}`, "bar"))
}

func TestOsDir(t *testing.T) {
	assert.NoError(t, runt(`{{ osDir "C:\\foo\\bar\\baz" }}`, "C:\\foo\\bar"))
}

func TestOsIsAbs(t *testing.T) {
	assert.NoError(t, runt(`{{ osIsAbs "C:\\foo" }}`, "true"))
	assert.NoError(t, runt(`{{ osIsAbs "foo" }}`, "false"))
}

func TestOsClean(t *testing.T) {
	assert.NoError(t, runt(`{{ osClean "C:\\foo\\..\\foo\\..\\bar" }}`, "C:\\bar"))
}

func TestOsExt(t *testing.T) {
	assert.NoError(t, runt(`{{ osExt "C:\\foo\\bar\\baz.txt" }}`, ".txt"))
}
