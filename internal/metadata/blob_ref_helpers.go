package metadata

import "github.com/lyonbrown4d/maxio/internal/model"

func cloneBlobRefPlacements(input []model.ShardPlacement) []model.ShardPlacement {
	if len(input) == 0 {
		return nil
	}
	output := make([]model.ShardPlacement, len(input))
	copy(output, input)
	return output
}

func cloneStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	output := make([]string, len(input))
	copy(output, input)
	return output
}

func cloneFirstInt64s(input [][]int64) []int64 {
	if len(input) == 0 {
		return nil
	}
	return cloneInt64s(input[0])
}

func cloneInt64s(input []int64) []int64 {
	if len(input) == 0 {
		return nil
	}
	output := make([]int64, len(input))
	copy(output, input)
	return output
}
