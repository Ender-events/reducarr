package scan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDuration_FormatHHMMSS(t *testing.T) {
	val, err := ParseDuration("01:30:15")
	assert.NoError(t, err)
	assert.Equal(t, float64(5415), val) // 1h (3600) + 30m (1800) + 15s
}

func TestParseDuration_FormatMMSS(t *testing.T) {
	val, err := ParseDuration("45:30")
	assert.NoError(t, err)
	assert.Equal(t, float64(2730), val) // 45m (2700) + 30s
}

func TestParseDuration_FormatSecondsOnly(t *testing.T) {
	val, err := ParseDuration("120")
	assert.NoError(t, err)
	assert.Equal(t, float64(120), val)
}

func TestParseDuration_Empty(t *testing.T) {
	val, err := ParseDuration("")
	assert.NoError(t, err)
	assert.Equal(t, float64(0), val)
}

func TestParseDuration_InvalidFormat(t *testing.T) {
	_, err := ParseDuration("1:2:3:4")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid duration format")

	_, err = ParseDuration("abc")
	assert.Error(t, err)
}
