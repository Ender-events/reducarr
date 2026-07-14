package scan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScorer_IsCandidate(t *testing.T) {
	s := &Scorer{
		MaxSize:    1024 * 1024 * 10, // 10 MiB
		MaxRatio:   5.0,              // 5 MiB/min
		MaxBitrate: 1000000,          // 1 Mbps (1,000,000 bps)
	}

	tests := []struct {
		name      string
		file      FileInfo
		candidate bool
		reason    string
	}{
		{
			name:      "Under all limits",
			file:      FileInfo{Size: 1024 * 1024 * 5, Duration: 120}, // 5 MiB, 2 min -> 2.5 MiB/min, bitrate is ~349 kbit/s
			candidate: false,
		},
		{
			name:      "Exceeds Size",
			file:      FileInfo{Size: 1024 * 1024 * 15, Duration: 300}, // 15 MiB, 5 min -> 3 MiB/min
			candidate: true,
			reason:    "exceeds",
		},
		{
			name:      "Exceeds Ratio",
			file:      FileInfo{Size: 1024 * 1024 * 8, Duration: 60}, // 8 MiB, 1 min -> 8 MiB/min
			candidate: true,
			reason:    "exceeds",
		},
		{
			name:      "Exceeds Bitrate",
			file:      FileInfo{Size: 1024 * 1024 * 2, Duration: 10}, // 2 MiB, 10s -> ~1.6 Mbps
			candidate: true,
			reason:    "exceeds",
		},
		{
			name:      "Duration is zero",
			file:      FileInfo{Size: 1024 * 1024 * 5, Duration: 0}, // 5 MiB, 0 duration -> no ratio/bitrate checks
			candidate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isCand, reason := s.IsCandidate(tt.file)
			assert.Equal(t, tt.candidate, isCand)
			if tt.candidate && tt.reason != "" {
				assert.Contains(t, reason, tt.reason)
			}
		})
	}
}

func TestParseBitrate(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"10Mbit", 10000000},
		{"500kbps", 500000},
		{"2000bps", 2000},
		{"1.5 Mbps", 1500000},
	}

	for _, tt := range tests {
		val, err := ParseBitrate(tt.input)
		assert.NoError(t, err)
		assert.Equal(t, tt.expected, val)
	}

	_, err := ParseBitrate("invalid")
	assert.Error(t, err)
}

func TestParseRatio(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"5mib/min", 5.0},
		{"10.5 MB/min", 10.5},
		{"8", 8.0},
	}

	for _, tt := range tests {
		val, err := ParseRatio(tt.input)
		assert.NoError(t, err)
		assert.Equal(t, tt.expected, val)
	}

	_, err := ParseRatio("invalid")
	assert.Error(t, err)
}
