package spectro

import (
	"testing"
	"time"
)

func TestEmitAndPartitions(t *testing.T) {
	r := New(time.Second, 100, 100)
	base := time.Unix(1_700_000_000, 0)
	r.Emit("a", base, nil, map[string]float64{"x": 1})
	r.Emit("a", base, nil, map[string]float64{"x": 2})
	r.Emit("b", base, nil, map[string]float64{"x": 3})

	parts := r.Partitions()
	if len(parts) != 2 || parts[0] != "a" || parts[1] != "b" {
		t.Errorf("partitions = %v, want [a b]", parts)
	}
}

func TestHeatmapCounts(t *testing.T) {
	r := New(time.Second, 100, 100)
	base := time.Unix(1_700_000_000, 0)
	r.Emit("p", base, nil, map[string]float64{"v": 0.1})                  // bucket 0, y-bin 0
	r.Emit("p", base, nil, map[string]float64{"v": 0.6})                  // bucket 0, y-bin 1
	r.Emit("p", base, nil, map[string]float64{"v": 0.9})                  // bucket 0, y-bin 1
	r.Emit("p", base.Add(time.Second), nil, map[string]float64{"v": 0.2}) // bucket 1, y-bin 0
	r.Emit("p", base.Add(time.Second), nil, map[string]float64{"v": 0.8}) // bucket 1, y-bin 1

	hm := r.Heatmap(HeatmapQuery{
		Partition: "p",
		From:      base,
		To:        base.Add(2 * time.Second),
		Y:         func(o Observation) float64 { return o.Metrics["v"] },
		YBins:     2,
		YMin:      0, YMax: 1,
	})
	if hm.XBins != 2 || hm.YBins != 2 {
		t.Fatalf("bins = %dx%d, want 2x2", hm.XBins, hm.YBins)
	}
	if hm.Cells[0][0] != 1 || hm.Cells[0][1] != 2 {
		t.Errorf("bucket 0: %v, want [1 2]", hm.Cells[0])
	}
	if hm.Cells[1][0] != 1 || hm.Cells[1][1] != 1 {
		t.Errorf("bucket 1: %v, want [1 1]", hm.Cells[1])
	}
}

func TestHeatmapFilter(t *testing.T) {
	r := New(time.Second, 100, 100)
	base := time.Unix(1_700_000_000, 0)
	r.Emit("p", base, map[string]string{"host": "a"}, map[string]float64{"v": 0.5})
	r.Emit("p", base, map[string]string{"host": "b"}, map[string]float64{"v": 0.5})

	hm := r.Heatmap(HeatmapQuery{
		Partition: "p",
		From:      base,
		To:        base.Add(time.Second),
		Filter:    func(o Observation) bool { return o.Dims["host"] == "a" },
		Y:         func(o Observation) float64 { return o.Metrics["v"] },
		YBins:     1,
		YMin:      0, YMax: 1,
	})
	if hm.Cells[0][0] != 1 {
		t.Errorf("filtered count = %d, want 1", hm.Cells[0][0])
	}
}

func TestBucketEviction(t *testing.T) {
	r := New(time.Second, 2, 100)
	base := time.Unix(1_700_000_000, 0)
	r.Emit("p", base, nil, map[string]float64{"v": 1})
	r.Emit("p", base.Add(time.Second), nil, map[string]float64{"v": 1})
	r.Emit("p", base.Add(2*time.Second), nil, map[string]float64{"v": 1})

	r.mu.Lock()
	defer r.mu.Unlock()
	if got := len(r.partitions["p"].buckets); got != 2 {
		t.Errorf("buckets after eviction = %d, want 2", got)
	}
	if _, ok := r.partitions["p"].buckets[base.UnixNano()]; ok {
		t.Error("oldest bucket was not evicted")
	}
}

func TestPerBucketCap(t *testing.T) {
	r := New(time.Second, 100, 2)
	base := time.Unix(1_700_000_000, 0)
	for i := 0; i < 5; i++ {
		r.Emit("p", base, nil, map[string]float64{"v": float64(i)})
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if got := len(r.partitions["p"].buckets[base.UnixNano()]); got != 2 {
		t.Errorf("obs after cap = %d, want 2", got)
	}
}

func TestReset(t *testing.T) {
	r := New(time.Second, 100, 100)
	r.Emit("p", time.Unix(1_700_000_000, 0), nil, map[string]float64{"v": 1})
	r.Reset()
	if got := r.Partitions(); len(got) != 0 {
		t.Errorf("after reset: %v, want empty", got)
	}
}
