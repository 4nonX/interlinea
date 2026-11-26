package pdf

import (
	"math"
	"testing"
)

// Blocks of a broken file, with extents that are no numbers, infinite,
// too large to part or upside down, are still put in an order: parting them never ends otherwise,
// and the reader runs out of stack, which ends the whole program.
func TestOrderOfBrokenBlocks(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	for name, blocks := range map[string][]*block{
		"not numbers": {{x0: nan, x1: nan, y0: nan, y1: nan}, {x0: 0, x1: 10, y0: 0, y1: 10}, {x0: 20, x1: 30, y0: 50, y1: 60}},
		"infinite":    {{x0: 0, x1: 10, y0: 0, y1: 10}, {x0: 0, x1: 10, y0: inf, y1: inf}},
		"too large":   {{x0: 0, x1: 1e17, y0: 0, y1: 10}, {x0: 1e17 + 16, x1: 2e17, y0: 0, y1: 10}},
		"upside down": {{x0: 0, x1: 10, y0: 40, y1: -40}, {x0: 0, x1: 10, y0: 10, y1: 5}, {x0: 50, x1: 40, y0: 30, y1: 20}},
	} {
		if got := inOrder(blocks); len(got) != len(blocks) {
			t.Errorf("%s: %d blocks of %d", name, len(got), len(blocks))
		}
	}
}
