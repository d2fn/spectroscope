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

func (ss *SpectroServer) Query(query SpectrogramQuery) (*Spectrogram, error) {
	if query.Measure == "" {
		return nil, fmt.Errorf("Measure is required")
	}
	yBins := query.YBins
	if yBins <= 0 {
		// 20 is a sensitlbe default i guess
		yBins = 20
	}

	// yeah go ahead and aggregate
	tbl, tableMax, found := ss.db.aggregate(query.Measure)
	
	bucketDur := time.Duration(tbl.alignment.blocksize) * time.Millisecond
	if !found {
		return &Spectrogram{YBins: yBins, BucketDur: bucketDur}, nil
	}

	// The ring buffer holds rows in arbitrary slot order; reorder by block
	// time so the x-axis runs oldest → newest.
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

	cells := make([][]int, xBins)
	for x, r := range populated {
		cells[x] = make([]int, yBins)
		for _, v := range r.values[:r.writePtr] {
			if math.IsNaN(v) || v < yMin || v >= yMax {
				continue
			}
			y := int((v - yMin) / (yMax - yMin) * float64(yBins))
			if y == yBins {
				y--
			}
			cells[x][y]++
		}
	}

	var from, to time.Time
	if xBins > 0 {
		from = time.UnixMilli(populated[0].block.from())
		to = time.UnixMilli(populated[xBins-1].block.to())
	}

	return &Spectrogram{
		From:      from,
		To:        to,
		BucketDur: bucketDur,
		XBins:     xBins,
		YBins:     yBins,
		YMin:      yMin,
		YMax:      yMax,
		Cells:     cells,
	}, nil
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

// HeatmapQuery parameterizes a Heatmap call.
type SpectrogramQuery struct {
	Measure    string
	YBins      int
}

type SpectrogramReply struct {
	Spectrogram Spectrogram `json:"spectrogram"`
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
