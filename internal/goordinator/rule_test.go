package goordinator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateQueryEscape(t *testing.T) {
	templFunc := renderFunc(&Event{})
	res, err := templFunc(`{{ queryescape "a&b+c" }}`)
	require.NoError(t, err)
	assert.Equal(t, "a%26b%2Bc", res)
}
