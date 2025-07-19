package sprig

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFail(t *testing.T) {
	const msg = "This is an error!"
	tpl := fmt.Sprintf(`{{fail "%s"}}`, msg)
	_, err := runRaw(tpl, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), msg)
}
