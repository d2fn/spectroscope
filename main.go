package main

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"time"

	"spectroserver/spectro"
)

// Demo wiring: construct a SpectroServer, mount its Handler under
// /spectrogram/ on an application-owned mux, and feed it synthetic
// observations. Hit:
//
//	curl 'http://127.0.0.1:6060/spectrogram/latency_ms'
//	curl 'http://127.0.0.1:6060/spectrogram/latency_ms?yBins=40'
func main() {
	ss := spectro.New(
		time.Second,                // precision: 1s buckets
		3600,                       // history: 1h of buckets
		[]string{"host"},           // dimensions
		[]string{"latency_ms"},     // measures
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ss.Start(ctx)
	go emitLoop(ss)

	mux := http.NewServeMux()
	mux.Handle("/spectrogram/", http.StripPrefix("/spectrogram", ss.Handler()))

	const addr = "127.0.0.1:6060"
	log.Printf("spectroserver listening on http://%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func emitLoop(ss *spectro.SpectroServer) {
	for {
		_ = ss.Emit(spectro.Observation{
			Time:       time.Now(),
			Dimensions: map[string]string{"host": "adf"},
			Measures:   map[string]float64{"latency_ms": rand.Float64() * 200},
		})
		time.Sleep(10 * time.Millisecond)
	}
}
