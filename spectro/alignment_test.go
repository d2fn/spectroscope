package spectro

import "testing"

func TestAlignmentAt(t *testing.T) {
	a := alignment{blocksize: 10}
	tests := []struct {
		value            int64
		wantFrom, wantTo int64
	}{
		{0, 0, 9},
		{5, 0, 9},
		{9, 0, 9},
		{10, 10, 19},
		{15, 10, 19},
		{99, 90, 99},
		{100, 100, 109},
	}
	for _, tt := range tests {
		b := a.at(tt.value)
		if b.from() != tt.wantFrom || b.to() != tt.wantTo {
			t.Errorf("at(%d) = (%d, %d), want (%d, %d)",
				tt.value, b.from(), b.to(), tt.wantFrom, tt.wantTo)
		}
	}
}

func TestBlockNextPrev(t *testing.T) {
	a := alignment{blocksize: 100}
	b := a.at(250)
	if b.from() != 200 || b.to() != 299 {
		t.Fatalf("at(250) = (%d, %d), want (200, 299)", b.from(), b.to())
	}
	if n := b.next(); n.from() != 300 || n.to() != 399 {
		t.Errorf("next = (%d, %d), want (300, 399)", n.from(), n.to())
	}
	if p := b.prev(); p.from() != 100 || p.to() != 199 {
		t.Errorf("prev = (%d, %d), want (100, 199)", p.from(), p.to())
	}
}

func TestBlockBeforeAfter(t *testing.T) {
	a := alignment{blocksize: 10}
	b0 := a.at(0)  // [0, 9]
	b1 := a.at(10) // [10, 19]

	if !b0.before(b1) {
		t.Error("b0 should be before b1")
	}
	if b1.before(b0) {
		t.Error("b1 should not be before b0")
	}
	if !b1.after(b0) {
		t.Error("b1 should be after b0")
	}
	if b0.after(b1) {
		t.Error("b0 should not be after b1")
	}
	if b0.before(b0) || b0.after(b0) {
		t.Error("a block is neither before nor after itself")
	}
}

func TestBlockOverlapsRange(t *testing.T) {
	a := alignment{blocksize: 10}
	b := a.at(50) // [50, 59]
	tests := []struct {
		from, to int64
		want     bool
	}{
		{50, 59, true},   // identical
		{45, 64, true},   // input contains b
		{52, 55, true},   // input contained in b
		{45, 50, true},   // left edge
		{59, 65, true},   // right edge
		{0, 49, false},   // before
		{60, 100, false}, // after
		{0, 100, true},   // spans b
	}
	for _, tt := range tests {
		if got := b.overlapsRange(tt.from, tt.to); got != tt.want {
			t.Errorf("overlapsRange(%d, %d) = %v, want %v",
				tt.from, tt.to, got, tt.want)
		}
	}
}

func TestBlockOverlapsBlock(t *testing.T) {
	a := alignment{blocksize: 10}
	if !a.at(50).overlaps(a.at(55)) {
		t.Error("[50,59] should overlap [50,59] (same block)")
	}
	if a.at(50).overlaps(a.at(70)) {
		t.Error("[50,59] should not overlap [70,79]")
	}
}

func TestAlignmentBlocks(t *testing.T) {
	a := alignment{blocksize: 10}
	bs := a.blocks(15, 33)
	wants := []struct{ from, to int64 }{{10, 19}, {20, 29}, {30, 39}}
	if len(bs) != len(wants) {
		t.Fatalf("len = %d, want %d", len(bs), len(wants))
	}
	for i, w := range wants {
		if bs[i].from() != w.from || bs[i].to() != w.to {
			t.Errorf("blocks[%d] = (%d, %d), want (%d, %d)",
				i, bs[i].from(), bs[i].to(), w.from, w.to)
		}
	}
}

func TestAlignmentBlocksSwap(t *testing.T) {
	a := alignment{blocksize: 10}
	forward := a.blocks(5, 25)
	reverse := a.blocks(25, 5)
	if len(forward) != len(reverse) {
		t.Fatalf("len forward=%d reverse=%d", len(forward), len(reverse))
	}
	for i := range forward {
		if forward[i] != reverse[i] {
			t.Errorf("blocks[%d] differ: %v vs %v", i, forward[i], reverse[i])
		}
	}
}

func TestBlockString(t *testing.T) {
	a := alignment{blocksize: 10}
	if got := a.at(50).String(); got != "(50, 59)" {
		t.Errorf("String = %q, want (50, 59)", got)
	}
}
