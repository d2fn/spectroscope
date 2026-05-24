package main

import (
	"context"
	"log"
	"math"
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
		600,                       // history: 1h of buckets
		[]string{"host"},           // dimensions
		[]string{"latency_ms"},     // measures
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ss.Start(ctx)
	go emitWave(ss, "a", 150, 60, 3, 180*time.Second)
	go emitWave(ss, "b", 10, 5, 1.5, 30*time.Second)
	go emitNoise(ss, "noise", 0, 200)
	go emitRamp(ss, "ramp", 200.0/60.0, 200)

	mux := http.NewServeMux()
	mux.Handle("/spectrogram/", http.StripPrefix("/spectrogram", ss.Handler()))

	const addr = "127.0.0.1:6060"
	log.Printf("spectroserver listening on http://%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func emitRamp(ss *spectro.SpectroServer, host string, slope, wrap float64) {
	start := time.Now()
	for {
		now := time.Now()
		elapsed := now.Sub(start).Seconds()
		v := math.Mod(elapsed*slope, wrap)
		_ = ss.Emit(spectro.Observation{
			Time:       now,
			Dimensions: map[string]string{"host": host},
			Measures:   map[string]float64{"latency_ms": v},
		})
		time.Sleep(10 * time.Millisecond)
	}
}

func emitNoise(ss *spectro.SpectroServer, host string, lo, hi float64) {
	for {
		_ = ss.Emit(spectro.Observation{
			Time:       time.Now(),
			Dimensions: map[string]string{"host": host},
			Measures:   map[string]float64{"latency_ms": lo + rand.Float64()*(hi-lo)},
		})
		time.Sleep(1 * time.Millisecond)
	}
}

func emitWave(ss *spectro.SpectroServer, host string, mean, amp, jitter float64, period time.Duration) {
	start := time.Now()
	for {
		now := time.Now()
		phase := 2 * math.Pi * float64(now.Sub(start)) / float64(period)
		v := mean + amp*math.Sin(phase) + rand.NormFloat64()*jitter
		_ = ss.Emit(spectro.Observation{
			Time:       now,
			Dimensions: map[string]string{"host": host},
			Measures:   map[string]float64{"latency_ms": v},
		})
		time.Sleep(10 * time.Millisecond)
	}
}
