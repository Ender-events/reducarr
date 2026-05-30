package scan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScorer_IsCandidate(t *testing.T) {
	s := &Scorer{
		MaxSize:  1024 * 1024 * 10, // 10 MiB
		MaxRatio: 5.0,              // 5 MiB/min
	}

	tests := []struct {
		name      string
		file      FileInfo
		candidate bool
	}{
		{
			name:      "Under both limits",
			file:      FileInfo{Size: 1024 * 1024 * 5, Duration: 120}, // 5 MiB, 2 min -> 2.5 MiB/min
			candidate: false,
		},
		{
			name:      "Exceeds Size",
			file:      FileInfo{Size: 1024 * 1024 * 15, Duration: 300}, // 15 MiB, 5 min -> 3 MiB/min
			candidate: true,
		},
		{
			name:      "Exceeds Ratio",
			file:      FileInfo{Size: 1024 * 1024 * 8, Duration: 60}, // 8 MiB, 1 min -> 8 MiB/min
			candidate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isCand, _ := s.IsCandidate(tt.file)
			assert.Equal(t, tt.candidate, isCand)
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
}
