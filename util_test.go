package gocron

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveSliceDuplicatesInt(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{
			"lots of duplicates",
			[]int{
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
				3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
				4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4,
				5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5,
			},
			[]int{1, 2, 3, 4, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeSliceDuplicatesInt(tt.input)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}

}
