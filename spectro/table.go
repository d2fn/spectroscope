package spectro

// defaultRowCapacity caps observations stored per time bucket. Excess emits
// to the same bucket are dropped (see table.emit). Memory cost per table is
// numRows * defaultRowCapacity * 8 bytes.
const defaultRowCapacity = 1024

type table struct {
	dimensions map[string]string
	alignment alignment
	rows []row
}

func newTable(dimensions map[string]string, alignment alignment, numRows int) *table {
	rows := make([]row, numRows)
	for i := range rows {
		rows[i].values = make([]float64, defaultRowCapacity)
	}
	return &table {
		dimensions: dimensions,
		alignment: alignment,
		rows: rows,
	}
}

type row struct {
	block block
	// next index to write to. if writePtr == len(data) then we are full
	writePtr int
	values []float64
	maxValue float64
	minValue float64
}


func (t *table) emit(ts int64, value float64) {
	b := t.alignment.at(ts)
	i := b.ringBufferIndex(len(t.rows))
	row := &t.rows[i]
	if row.block.before(b) {
		// we're starting a new time block
		// reset write ptr and set to new time
		row.block = b
		row.writePtr = 0
		row.maxValue = 0
		row.minValue = 0
	}
	if row.writePtr == len(row.values) {
		// either drop data silently or write over a random position in the value array
		// drop for now
	} else {
		row.values[row.writePtr] = value
		row.maxValue = max(row.maxValue, value)
		row.minValue = min(row.minValue, value)
		row.writePtr++
	}
}

func (t *table) addAll(in *table) float64 {
	var maxValue float64
	if len(t.rows) != len(in.rows) {
		return maxValue
	}
	for i := range len(in.rows) {
		row := in.rows[i]
		for _, value := range row.values[:row.writePtr] {
			t.emit(row.block.start, value)
		}
		// get the max value from the newly written row
		maxValue = max(maxValue, t.rows[i].maxValue)
	}
	return maxValue
}
