package scan

import (
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"
)

type Scorer struct {
	MaxSize    uint64  // bytes
	MaxRatio   float64 // MiB per minute
	MaxBitrate int64   // bits per second
}

type FileInfo struct {
	Size     int64   // bytes
	Duration float64 // seconds
}

func (s *Scorer) IsCandidate(f FileInfo) (bool, string) {
	if s.MaxSize > 0 && uint64(f.Size) > s.MaxSize {
		return true, fmt.Sprintf("Size %s exceeds %s", humanize.Bytes(uint64(f.Size)), humanize.Bytes(s.MaxSize))
	}

	if f.Duration > 0 {
		// Calculate MiB/min
		mib := float64(f.Size) / (1024 * 1024)
		minutes := f.Duration / 60
		ratio := mib / minutes

		if s.MaxRatio > 0 && ratio > s.MaxRatio {
			return true, fmt.Sprintf("Ratio %.2f MiB/min exceeds %.2f MiB/min", ratio, s.MaxRatio)
		}

		// Calculate bitrate (bits/s)
		bitrate := int64(float64(f.Size*8) / f.Duration)
		if s.MaxBitrate > 0 && bitrate > s.MaxBitrate {
			return true, fmt.Sprintf("Bitrate %s exceeds %s", humanize.SIWithDigits(float64(bitrate), 2, "bps"), humanize.SIWithDigits(float64(s.MaxBitrate), 2, "bps"))
		}
	}

	return false, ""
}

func ParseRatio(s string) (float64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, "mib/min")
	s = strings.TrimSuffix(s, "mb/min")
	var val float64
	_, err := fmt.Sscanf(s, "%f", &val)
	return val, err
}

func ParseBitrate(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	// Simple multiplier logic
	multiplier := int64(1)
	if strings.HasSuffix(s, "mbit") || strings.HasSuffix(s, "mbps") {
		multiplier = 1000 * 1000
		s = s[:len(s)-4]
	} else if strings.HasSuffix(s, "kbit") || strings.HasSuffix(s, "kbps") {
		multiplier = 1000
		s = s[:len(s)-4]
	} else if strings.HasSuffix(s, "bit") || strings.HasSuffix(s, "bps") {
		s = s[:len(s)-3]
	}

	var val float64
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%f", &val)
	if err != nil {
		return 0, err
	}
	return int64(val * float64(multiplier)), nil
}
