package fsutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapPath_RewritesKnownPrefix(t *testing.T) {
	mappings := []PathMapping{
		{Remote: "/remote/path", Local: "/local/path"},
	}
	res := MapPath("/remote/path/to/file.txt", mappings)
	assert.Equal(t, "/local/path/to/file.txt", res)
}

func TestMapPath_ReturnsOriginalIfNoMatch(t *testing.T) {
	mappings := []PathMapping{
		{Remote: "/remote/path", Local: "/local/path"},
	}
	res := MapPath("/other/path/to/file.txt", mappings)
	assert.Equal(t, "/other/path/to/file.txt", res)
}

func TestMapPath_EmptyMappings(t *testing.T) {
	res := MapPath("/remote/path/to/file.txt", nil)
	assert.Equal(t, "/remote/path/to/file.txt", res)
}

func TestMapPath_UsesFirstMatchingMapping(t *testing.T) {
	mappings := []PathMapping{
		{Remote: "/remote/path/sub", Local: "/local/path/one"},
		{Remote: "/remote/path", Local: "/local/path/two"},
	}
	res := MapPath("/remote/path/sub/file.txt", mappings)
	assert.Equal(t, "/local/path/one/file.txt", res)
}
