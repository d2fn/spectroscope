package spectro

type database struct {
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
			tmap[hash] = newTable(obs.Dimensions, db.alignment, db.numRows)
		}
		t := tmap[hash]
		t.emit(obs.Time.UnixMilli(), value)
	}
}

func (db *database) aggregate(metric string) (*table, float64, bool) {
	var tableMax float64
	aggTable := newTable(map[string]string{}, db.alignment, db.numRows)
	found := false
	if tables, ok := db.data[metric]; ok {
		for _, table := range tables {
			localMax := aggTable.addAll(table)
			tableMax = max(localMax, tableMax)
			found = true
		}
	}
	return aggTable, tableMax, found
}

