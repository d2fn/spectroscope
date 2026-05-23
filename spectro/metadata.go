package spectro

import (
	"encoding/binary"
	"hash/fnv"
)

type fields struct {
	names []string
	positions map[string]int
}

func newFields(names []string) *fields {
	return &fields {
		names: names,
		positions: positionMap(names),
	}
}

func positionMap(names []string) map[string]int {
	out := make(map[string]int)
	for i, f := range names {
		out[f] = i
	}
	return out
}

func fieldValues[V any](f *fields, data map[string]V) []V {
	values := make([]V, len(f.names))
	for k, v := range data {
		values[f.positions[k]] = v
	}
	return values
}

// fieldHash returns an FNV-64a digest over data's values laid out in
// field-position order. Two maps with the same key/value pairs produce the
// same hash. Each value is length-prefixed so that position boundaries are
// unambiguous (e.g. {"a":"bc"} and {"a":"b","b":"c"} can't collide).
func fieldHash(f *fields, data map[string]string) uint64 {
	values := fieldValues(f, data)
	h := fnv.New64a()
	var lenBuf [4]byte
	for _, v := range values {
		binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(v)))
		h.Write(lenBuf[:])
		h.Write([]byte(v))
	}
	return h.Sum64()
}



