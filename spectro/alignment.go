package spectro

import "fmt"

// alignment models a layout of int64 values into fixed-width blocks (of 10,
// 100, 1000, etc). Ported from passage's com.d2fn.passage.math.Alignment.
type alignment struct {
	blocksize int64
}

// at returns the block containing i.
func (a alignment) at(i int64) block {
	return block{a: a, start: i / a.blocksize * a.blocksize}
}

// blocks returns the blocks covering the inclusive range [from, to]. If
// from > to, the arguments are swapped.
func (a alignment) blocks(from, to int64) []block {
	if from > to {
		from, to = to, from
	}
	out := make([]block, 0, (to-from)/a.blocksize+2)
	b := a.at(from)
	out = append(out, b)
	for b.to() < to {
		b = b.next()
		out = append(out, b)
	}
	return out
}

// block is a fixed-width range produced by an alignment. Ported from
// passage's com.d2fn.passage.math.Block.
type block struct {
	a     alignment
	start int64
}

func (b block) next() block { return block{a: b.a, start: b.start + b.a.blocksize} }
func (b block) prev() block { return block{a: b.a, start: b.start - b.a.blocksize} }

func (b block) ringBufferIndex(numBlocks int) int {
	return int((b.start / b.a.blocksize) % int64(numBlocks))
}

// from returns the inclusive lower bound.
func (b block) from() int64 { return b.start }

// to returns the inclusive upper bound.
func (b block) to() int64 { return b.start + b.a.blocksize - 1 }

// before reports whether b is ordered before other (compares start positions).
func (b block) before(other block) bool { return b.start < other.start }

// after reports whether b is ordered after other (compares start positions).
func (b block) after(other block) bool { return b.start > other.start }

// overlaps reports whether b intersects other.
func (b block) overlaps(other block) bool {
	return b.overlapsRange(other.from(), other.to())
}

// overlapsRange reports whether b intersects the inclusive range [from, to].
func (b block) overlapsRange(from, to int64) bool {
	return from <= b.to() && b.from() <= to
}

func (b block) String() string {
	return fmt.Sprintf("(%d, %d)", b.from(), b.to())
}
