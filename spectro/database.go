package spectro

import (
	"math"
	"sync"
)

type database struct {
	mu sync.RWMutex
	dimensions *fields
	alignment alignment
	numRows int
	// top level key is metric name
	// second level key is the hash of the dimensions
	data map[string]map[uint64]*table
}

func newDatabase(dimensions *fields, alignment alignment, numRows int) *database {
	return &database {
		dimensions: dimensions,
		alignment: alignment,
		numRows: numRows,
		data: make(map[string]map[uint64]*table),
	}
}

func (db *database) write(obs Observation) {
	db.mu.Lock()
	defer db.mu.Unlock()
	// get the dimension hash we'll use to select the database under each metric
	hash := fieldHash(db.dimensions, obs.Dimensions)
	// record each metric in observation
	for measure, value := range obs.Measures {
		if db.data[measure] == nil {
			// add tables for metric if necessary
			db.data[measure] = make(map[uint64]*table)
		}
		tmap := db.data[measure]
		if tmap[hash] == nil {
			tmap[hash] = newTable(obs.Dimensions, db.alignment, db.numRows, defaultRowCapacity, false)
		}
		t := tmap[hash]
		t.emit(obs.Time.UnixMilli(), value)
	}
}

func (db *database) aggregate(metric string) (*table, float64, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	var tableMax float64
	aggTable := newTable(map[string]string{}, db.alignment, db.numRows, defaultRowCapacity, true)
	found := false
	if tables, ok := db.data[metric]; ok {
		for _, table := range tables {
			var srcPopulated, srcTotal int
			var srcMin, srcMax float64
			srcMin = math.Inf(1)
			for i := range table.rows {
				if table.rows[i].writePtr > 0 {
					srcPopulated++
					srcTotal += table.rows[i].writePtr
					if table.rows[i].maxValue > srcMax {
						srcMax = table.rows[i].maxValue
					}
					if table.rows[i].minValue < srcMin {
						srcMin = table.rows[i].minValue
					}
				}
			}
			localMax := aggTable.addAll(table)
			tableMax = max(localMax, tableMax)
			found = true
		}
		var aggPopulated, aggTotal int
		for i := range aggTable.rows {
			if aggTable.rows[i].writePtr > 0 {
				aggPopulated++
				aggTotal += aggTable.rows[i].writePtr
			}
		}
	}
	return aggTable, tableMax, found
}

// aggregateBy buckets the per-dimension tables by the value of one dimension
// and folds each bucket into a single table. The returned map is keyed by
// that value (e.g. groupBy="host" → keys "a", "b", "noise", ...). tableMax
// is the global max so every group shares the same y-scale.
func (db *database) aggregateBy(metric, dim string) (map[string]*table, float64, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	groups := make(map[string]*table)
	var tableMax float64
	found := false
	tables, ok := db.data[metric]
	if !ok {
		return groups, 0, false
	}
	for _, t := range tables {
		val := t.dimensions[dim]
		agg, exists := groups[val]
		if !exists {
			agg = newTable(map[string]string{dim: val}, db.alignment, db.numRows, defaultRowCapacity, true)
			groups[val] = agg
		}
		localMax := agg.addAll(t)
		tableMax = max(tableMax, localMax)
		found = true
	}
	return groups, tableMax, found
}

