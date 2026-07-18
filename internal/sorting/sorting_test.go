package sorting

import (
	"math"
	"testing"
)

// testReleaseLike is a simple implementation of ReleaseLike for testing
type testReleaseLike struct {
	rejections []string
	score      int32
	size       int64
}

func (t testReleaseLike) GetRejections() []string { return t.rejections }
func (t testReleaseLike) GetScore() int32         { return t.score }
func (t testReleaseLike) GetSize() int64          { return t.size }

func TestSort(t *testing.T) {
	tests := []struct {
		name     string
		input    []testReleaseLike
		expected []testReleaseLike
	}{
		{
			name: "Approved releases come before rejected",
			input: []testReleaseLike{
				{rejections: []string{"bad"}, score: 100, size: 1000},
				{rejections: []string{}, score: 50, size: 2000},
			},
			expected: []testReleaseLike{
				{rejections: []string{}, score: 50, size: 2000},
				{rejections: []string{"bad"}, score: 100, size: 1000},
			},
		},
		{
			name: "Higher score comes first",
			input: []testReleaseLike{
				{rejections: []string{}, score: 50, size: 1000},
				{rejections: []string{}, score: 100, size: 2000},
			},
			expected: []testReleaseLike{
				{rejections: []string{}, score: 100, size: 2000},
				{rejections: []string{}, score: 50, size: 1000},
			},
		},
		{
			name: "Smaller size comes first when scores are equal",
			input: []testReleaseLike{
				{rejections: []string{}, score: 100, size: 2000},
				{rejections: []string{}, score: 100, size: 1000},
			},
			expected: []testReleaseLike{
				{rejections: []string{}, score: 100, size: 1000},
				{rejections: []string{}, score: 100, size: 2000},
			},
		},
		{
			name: "Lower severity rejection comes first",
			input: []testReleaseLike{
				{rejections: []string{"does not allow upgrades"}, score: 100, size: 1000},
				{rejections: []string{"bad quality"}, score: 100, size: 1000},
			},
			expected: []testReleaseLike{
				{rejections: []string{"does not allow upgrades"}, score: 100, size: 1000},
				{rejections: []string{"bad quality"}, score: 100, size: 1000},
			},
		},
		{
			name: "Worst severity (Unknown Movie) comes last",
			input: []testReleaseLike{
				{rejections: []string{"Unknown Movie"}, score: 100, size: 1000},
				{rejections: []string{"bad quality"}, score: 100, size: 1000},
			},
			expected: []testReleaseLike{
				{rejections: []string{"bad quality"}, score: 100, size: 1000},
				{rejections: []string{"Unknown Movie"}, score: 100, size: 1000},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of input to avoid modifying the original
			input := make([]testReleaseLike, len(tt.input))
			copy(input, tt.input)

			Sort(input)

			if len(input) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d", len(input), len(tt.expected))
				return
			}

			for i := range input {
				if input[i].score != tt.expected[i].score ||
					input[i].size != tt.expected[i].size ||
					len(input[i].rejections) != len(tt.expected[i].rejections) {
					t.Errorf("mismatch at index %d:\ngot:  %+v\nwant: %+v", i, input[i], tt.expected[i])
				}
			}
		})
	}
}

func TestRejectionSeverity(t *testing.T) {
	tests := []struct {
		rejections []string
		want       int
	}{
		{nil, 0},
		{[]string{}, 0},
		{[]string{"does not allow upgrades"}, 1},
		{[]string{"equal or higher preference"}, 1},
		{[]string{"bad quality"}, 2},
		{[]string{"Unknown Movie"}, 3},
		{[]string{"Wrong episode"}, 3},
		{[]string{"bad quality", "does not allow upgrades"}, 2},
	}

	for _, tt := range tests {
		got := RejectionSeverity(tt.rejections)
		if got != tt.want {
			t.Errorf("RejectionSeverity(%v) = %d, want %d", tt.rejections, got, tt.want)
		}
	}
}

func TestGetScore(t *testing.T) {
	tests := []struct {
		name string
		in   *int32
		want int32
	}{
		{"nil", nil, math.MinInt32},
		{"zero", new(int32(0)), 0},
		{"positive", new(int32(100)), 100},
		{"negative", new(int32(-50)), -50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetScore(tt.in)
			if got != tt.want {
				t.Errorf("GetScore(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
