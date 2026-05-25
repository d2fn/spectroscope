// Package spectro is an in-process metric recorder that exposes a spectrogram-style
// heatmap view over emitted observations. Think pprof, but for tuples of
// (partition, timestamp, dims, metrics) where a math function over the metrics
// defines a y-axis you can render as heat density over time.
package spectro

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type SpectroServer struct {
	in chan Observation
	clock *clock
	dropped *atomic.Uint64
	dimensions *fields
	measures *fields
	db *database
}

// Observation is one emitted tuple: a set of dimension labels and metric values.
type Observation struct {
	Time time.Time
	Dimensions map[string]string
	Measures map[string]float64
}

// return a new spectrogram server with the given time precision (e.g. 1 * time.Second for 1s resolution)
// and `history` number of those precision blocks
func New(precision time.Duration, history int, dimensions []string, measures []string) *SpectroServer {
	dimensionFields := newFields(dimensions)
	return &SpectroServer {
		clock: &clock {},
		in: make(chan Observation, 10000),
		dropped: &atomic.Uint64 {},
		dimensions: dimensionFields,
		measures: newFields(measures),
		db: newDatabase(dimensionFields, alignment { 1 * time.Second.Milliseconds() }, history),
	}
}

func (ss *SpectroServer) Start(ctx context.Context) {
	for {
		select {
		case obs := <-ss.in:
			ss.db.write(obs)
		case <-ctx.Done():
			break
		}
	}
}

func (ss *SpectroServer) Emit(obs Observation) error {
	_, err := ss.clock.emit(obs.Time)
	if err != nil {
		return err
	}
	select {
	case ss.in <- obs:
		// accepted
	default:
		// increment metric on message drop from full queue
		ss.dropped.Add(1)
	}
	return nil
}

func (ss *SpectroServer) Query(query SpectrogramQuery) ([]SpectrogramGroup, error) {
	if query.Measure == "" {
		return nil, fmt.Errorf("Measure is required")
	}
	yBins := query.YBins
	if yBins <= 0 {
		yBins = 20
	}

	if query.GroupBy == "" {
		tbl, tableMax, _ := ss.db.aggregate(query.Measure)
		g := buildGroup(tbl, tableMax, yBins)
		g.Label = "all"
		return []SpectrogramGroup{g}, nil
	}

	if _, ok := ss.dimensions.positions[query.GroupBy]; !ok {
		return nil, fmt.Errorf("unknown dimension %q", query.GroupBy)
	}
	tables, tableMax, _ := ss.db.aggregateBy(query.Measure, query.GroupBy)

	labels := make([]string, 0, len(tables))
	for k := range tables {
		labels = append(labels, k)
	}
	sort.Strings(labels)

	out := make([]SpectrogramGroup, 0, len(labels))
	for _, label := range labels {
		g := buildGroup(tables[label], tableMax, yBins)
		g.Label = query.GroupBy + "=" + label
		out = append(out, g)
	}
	return out, nil
}

func buildGroup(tbl *table, tableMax float64, yBins int) SpectrogramGroup {
	bucketDur := time.Duration(tbl.alignment.blocksize) * time.Millisecond

	populated := make([]*row, 0, len(tbl.rows))
	for i := range tbl.rows {
		r := &tbl.rows[i]
		if r.writePtr > 0 {
			populated = append(populated, r)
		}
	}
	sort.Slice(populated, func(i, j int) bool {
		return populated[i].block.before(populated[j].block)
	})

	xBins := len(populated)
	yMin, yMax := 0.0, tableMax
	if yMax <= yMin {
		yMax = yMin + 1
	}

	totalValues := 0
	for _, r := range populated {
		totalValues += r.writePtr
	}
	allValues := make([]float64, 0, totalValues)

	cells := make([][]int, xBins)
	histCounts := make([]int, yBins)
	for x, r := range populated {
		cells[x] = make([]int, yBins)
		for _, v := range r.values[:r.writePtr] {
			if math.IsNaN(v) {
				continue
			}
			allValues = append(allValues, v)
			if v < yMin || v >= yMax {
				continue
			}
			y := int((v - yMin) / (yMax - yMin) * float64(yBins))
			if y == yBins {
				y--
			}
			cells[x][y]++
			histCounts[y]++
		}
	}

	var from, to time.Time
	if xBins > 0 {
		from = time.UnixMilli(populated[0].block.from())
		to = time.UnixMilli(populated[xBins-1].block.to())
	}

	sort.Float64s(allValues)

	return SpectrogramGroup{
		Spectrogram: Spectrogram{
			From:      from,
			To:        to,
			BucketDur: bucketDur,
			XBins:     xBins,
			YBins:     yBins,
			YMin:      yMin,
			YMax:      yMax,
			Cells:     cells,
		},
		Histogram: Histogram{
			YBins:  yBins,
			YMin:   yMin,
			YMax:   yMax,
			Counts: histCounts,
		},
		Percentiles: Percentiles{
			P50: percentile(allValues, 0.50),
			P75: percentile(allValues, 0.75),
			P99: percentile(allValues, 0.99),
		},
	}
}

// percentile uses nearest-rank on a sorted slice: index = floor(p*n),
// clamped to n-1. Returns 0 for an empty input.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	idx := int(p * float64(n))
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}

// a monotonic clock
type clock struct {
	mu sync.Mutex
	now time.Time
}

// cas to a new time as long as it does cause drift
// returns the prior observation time and an error if the emitted time would cause clock drift
// into the past
func (c *clock) emit(ts time.Time) (time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	poll := c.now
	if poll.After(ts) {
		return poll, fmt.Errorf("Clock drift detected")
	}
	c.now = ts
	return poll, nil
}

type SpectrogramQuery struct {
	Measure string
	YBins   int
	// GroupBy is the name of a dimension to split on. Empty means a single
	// aggregate across all dimension values.
	GroupBy string
}

type SpectrogramGroup struct {
	Label       string      `json:"label"`
	Spectrogram Spectrogram `json:"spectrogram"`
	Histogram   Histogram   `json:"histogram"`
	Percentiles  Percentiles `json:"percentiles"`
}

type SpectrogramReply struct {
	Groups []SpectrogramGroup `json:"groups"`
}

// Heatmap is a 2D histogram: Cells[xIdx][yIdx] is the count of observations
// that fell in time bucket xIdx and y-value bin yIdx.
type Spectrogram struct {
	From, To   time.Time
	BucketDur  time.Duration
	XBins      int
	YBins      int
	YMin, YMax float64
	Cells      [][]int
}

// Histogram is the per-y-bin count of observations aggregated across all
// x-bins in the spectrogram. Counts[i] is the number of values that fell
// in y-bin i; the bin's value range is [YMin+i*w, YMin+(i+1)*w) where
// w = (YMax-YMin)/YBins.
type Histogram struct {
	YBins      int
	YMin, YMax float64
	Counts     []int
}

// Percentiles computed over every non-NaN value contributing to the
// spectrogram, regardless of whether it fell inside [YMin, YMax).
type Percentiles struct {
	P50, P75, P99 float64
}

