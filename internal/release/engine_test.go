package release

import (
	"math"
	"testing"
)

func TestEngine_Sort(t *testing.T) {
	score10 := int32(10)
	score5 := int32(5)
	scoreMinus5 := int32(-5)

	releases := []Release{
		{
			Title:      "Release A",
			Size:       1000,
			Rejections: []string{"Some rejection"},
			Score:      &score10,
		},
		{
			Title:      "Release B",
			Size:       800,
			Rejections: []string{},
			Score:      &score5,
		},
		{
			Title:      "Release C",
			Size:       500,
			Rejections: []string{},
			Score:      &score10,
		},
		{
			Title:      "Release D",
			Size:       600,
			Rejections: []string{},
			Score:      &score10,
		},
		{
			Title:      "Release E",
			Size:       300,
			Rejections: []string{"Quality profile does not allow upgrades"}, // severity 1
			Score:      &scoreMinus5,
		},
		{
			Title:      "Release F",
			Size:       200,
			Rejections: []string{"Unknown Movie"}, // severity 3
			Score:      &score10,
		},
	}

	engine := NewEngine("10%", 0)
	engine.Sort(releases)

	// Expected order:
	// 1. Release C (Approved, score 10, size 500)
	// 2. Release D (Approved, score 10, size 600)
	// 3. Release B (Approved, score 5, size 800)
	// 4. Release A (Severity 2 rejection, score 10, size 1000)
	// 5. Release F (Severity 3 rejection, score 10, size 200)
	// 6. Release E (Severity 1 rejection, score -5, size 300)

	expectedOrder := []string{"Release C", "Release D", "Release B", "Release A", "Release F", "Release E"}
	for i, r := range releases {
		if r.Title != expectedOrder[i] {
			t.Errorf("At index %d, expected %s, got %s", i, expectedOrder[i], r.Title)
		}
	}
}

func TestEngine_EvaluateUpgrade(t *testing.T) {
	score10 := int32(10)
	score5 := int32(5)

	tests := []struct {
		name            string
		engine          *Engine
		best            Release
		currentSize     int64
		currentScore    int32
		expectedQualify bool
		expectedReason  string
	}{
		{
			name:   "Successful upgrade with percentage reduction",
			engine: NewEngine("10%", 0),
			best: Release{
				Size:       900,
				Protocol:   "torrent",
				Seeders:    5,
				Rejections: []string{},
				Score:      &score10,
			},
			currentSize:     1000,
			currentScore:    10,
			expectedQualify: true,
		},
		{
			name:   "Failed upgrade due to insufficient percentage reduction",
			engine: NewEngine("15%", 0),
			best: Release{
				Size:       900,
				Protocol:   "torrent",
				Seeders:    5,
				Rejections: []string{},
				Score:      &score10,
			},
			currentSize:     1000,
			currentScore:    10,
			expectedQualify: false,
			expectedReason:  "size reduction (10.00%) is below minimum required (15%)",
		},
		{
			name:   "Successful upgrade with absolute size reduction",
			engine: NewEngine("100mib", 0),
			best: Release{
				Size:       100 * 1024 * 1024,
				Protocol:   "torrent",
				Seeders:    5,
				Rejections: []string{},
				Score:      &score10,
			},
			currentSize:     250 * 1024 * 1024,
			currentScore:    10,
			expectedQualify: true,
		},
		{
			name:   "Failed upgrade due to insufficient absolute size reduction",
			engine: NewEngine("200mib", 0),
			best: Release{
				Size:       100 * 1024 * 1024,
				Protocol:   "torrent",
				Seeders:    5,
				Rejections: []string{},
				Score:      &score10,
			},
			currentSize:     250 * 1024 * 1024,
			currentScore:    10,
			expectedQualify: false,
			expectedReason:  "size reduction (157286400 bytes) is below minimum required (209715200 bytes)",
		},
		{
			name:   "Failed upgrade due to lower custom format score",
			engine: NewEngine("10%", 0),
			best: Release{
				Size:       800,
				Protocol:   "torrent",
				Seeders:    5,
				Rejections: []string{},
				Score:      &score5,
			},
			currentSize:     1000,
			currentScore:    10,
			expectedQualify: false,
			expectedReason:  "release custom format score (5) is lower than current score (10)",
		},
		{
			name:   "Successful upgrade even with lower score if current score is MinInt32",
			engine: NewEngine("10%", 0),
			best: Release{
				Size:       800,
				Protocol:   "torrent",
				Seeders:    5,
				Rejections: []string{},
				Score:      &score5,
			},
			currentSize:     1000,
			currentScore:    math.MinInt32,
			expectedQualify: true,
		},
		{
			name:   "Failed upgrade due to severe rejection",
			engine: NewEngine("10%", 0),
			best: Release{
				Size:       800,
				Protocol:   "torrent",
				Seeders:    5,
				Rejections: []string{"Unknown Movie"},
				Score:      &score10,
			},
			currentSize:     1000,
			currentScore:    10,
			expectedQualify: false,
			expectedReason:  "release has severe rejection(s): Unknown Movie",
		},
		{
			name:   "Successful upgrade with light rejection (severity 1)",
			engine: NewEngine("10%", 0),
			best: Release{
				Size:       800,
				Protocol:   "torrent",
				Seeders:    5,
				Rejections: []string{"Quality profile does not allow upgrades"},
				Score:      &score10,
			},
			currentSize:     1000,
			currentScore:    10,
			expectedQualify: true,
		},
		{
			name:   "Failed upgrade due to low seeders",
			engine: NewEngine("10%", 10),
			best: Release{
				Size:       800,
				Protocol:   "torrent",
				Seeders:    5,
				Rejections: []string{},
				Score:      &score10,
			},
			currentSize:     1000,
			currentScore:    10,
			expectedQualify: false,
			expectedReason:  "seeders (5) is below minimum allowed (10)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qualify, reason := tt.engine.EvaluateUpgrade(tt.best, tt.currentSize, tt.currentScore)
			if qualify != tt.expectedQualify {
				t.Errorf("expected qualify %v, got %v", tt.expectedQualify, qualify)
			}
			if !qualify && reason != tt.expectedReason {
				t.Errorf("expected reason %q, got %q", tt.expectedReason, reason)
			}
		})
	}
}
