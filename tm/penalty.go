package tm

import (
	"context"
	"fmt"

	"github.com/4nonX/interlinea/content"
)

// MaxPenalty is the largest penalty a memory can be given.
const MaxPenalty = 100

// Penalized returns m with every match score lowered by points, as CAT
// tools do for memories that are less trusted (an old client memory, a
// reference memory from another domain). A 100% match from a memory with
// a 1-point penalty scores 99%, so it is never inserted or counted as an
// exact match. Everything but Lookup passes through unchanged. A penalty
// of 0 returns m itself.
func Penalized(m Memory, points int) (Memory, error) {
	if points < 0 || points > MaxPenalty {
		return nil, fmt.Errorf("tm: penalty must be between 0 and %d, not %d", MaxPenalty, points)
	}
	if points == 0 {
		return m, nil
	}
	return &penalized{Memory: m, points: points}, nil
}

type penalized struct {
	Memory
	points int
}

// Penalty returns the penalty a memory from Penalized carries (0 for any
// other memory).
func Penalty(m Memory) int {
	if p, ok := m.(*penalized); ok {
		return p.points
	}
	return 0
}

// Lookup asks the memory for matches good enough to stay at or above the
// minimum after the penalty, then lowers their scores.
func (p *penalized) Lookup(ctx context.Context, source content.Content, opts LookupOptions) ([]Match, error) {
	opts = opts.withDefaults()
	min := opts.MinScore + p.points
	if min > 100 {
		return nil, nil
	}
	ms, err := p.Memory.Lookup(ctx, source, LookupOptions{MinScore: min, Limit: opts.Limit})
	if err != nil {
		return nil, err
	}
	out := make([]Match, len(ms))
	for i, x := range ms {
		x.Score -= p.points
		out[i] = x
	}
	return out, nil
}
