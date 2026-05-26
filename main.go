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
		[]string{"caller"},           // dimensions
		[]string{"latency_ms"},     // measures
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ss.Start(ctx)
	go emitMultiWave(ss, "a", []wave{
		// Slow base oscillation: large amplitude, period drifts on a 4-minute cycle.
		{mean: 100, baseAmp: 40, ampSwing: 20, ampDriftSecs: 90, baseFreqHz: 1.0 / 180.0, freqSwingHz: 1.0 / 600.0, freqDriftSecs: 240},
		// Medium-frequency layer.
		{mean: 0, baseAmp: 15, ampSwing: 8, ampDriftSecs: 50, baseFreqHz: 1.0 / 45.0, freqSwingHz: 1.0 / 120.0, freqDriftSecs: 80},
		// Fast ripple that breathes in and out.
		{mean: 0, baseAmp: 8, ampSwing: 4, ampDriftSecs: 25, baseFreqHz: 1.0 / 12.0, freqSwingHz: 1.0 / 40.0, freqDriftSecs: 60},
	}, 12)
	go emitMultiWave(ss, "b", []wave{
		{mean: 80, baseAmp: 25, ampSwing: 12, ampDriftSecs: 70, baseFreqHz: 1.0 / 120.0, freqSwingHz: 1.0 / 360.0, freqDriftSecs: 180},
		{mean: 0, baseAmp: 10, ampSwing: 5, ampDriftSecs: 40, baseFreqHz: 1.0 / 30.0, freqSwingHz: 1.0 / 90.0, freqDriftSecs: 90},
	}, 8)
	go emitMultiWave(ss, "c", []wave{
		// Stately high-mean carrier with very slow tempo drift.
		{mean: 150, baseAmp: 35, ampSwing: 15, ampDriftSecs: 120, baseFreqHz: 1.0 / 300.0, freqSwingHz: 1.0 / 900.0, freqDriftSecs: 360},
		{mean: 0, baseAmp: 12, ampSwing: 6, ampDriftSecs: 80, baseFreqHz: 1.0 / 60.0, freqSwingHz: 1.0 / 200.0, freqDriftSecs: 120},
	}, 10)
	go emitMultiWave(ss, "d", []wave{
		// Faster, busier signature — three layers, each in its own band.
		{mean: 60, baseAmp: 20, ampSwing: 10, ampDriftSecs: 40, baseFreqHz: 1.0 / 80.0, freqSwingHz: 1.0 / 200.0, freqDriftSecs: 100},
		{mean: 0, baseAmp: 12, ampSwing: 6, ampDriftSecs: 25, baseFreqHz: 1.0 / 20.0, freqSwingHz: 1.0 / 60.0, freqDriftSecs: 50},
		{mean: 0, baseAmp: 6, ampSwing: 3, ampDriftSecs: 15, baseFreqHz: 1.0 / 6.0, freqSwingHz: 1.0 / 15.0, freqDriftSecs: 30},
	}, 10)
	go emitMultiWave(ss, "e", []wave{
		// Bursty character — the second layer's amp can swing through
		// zero (ampSwing close to baseAmp) so it goes nearly silent and
		// then comes roaring back.
		{mean: 40, baseAmp: 15, ampSwing: 8, ampDriftSecs: 60, baseFreqHz: 1.0 / 100.0, freqSwingHz: 1.0 / 300.0, freqDriftSecs: 150},
		{mean: 0, baseAmp: 25, ampSwing: 22, ampDriftSecs: 90, baseFreqHz: 1.0 / 25.0, freqSwingHz: 1.0 / 70.0, freqDriftSecs: 60},
	}, 6)
	go emitNoise(ss, "noise", 0, 250)
	//go emitRamp(ss, "ramp", 200.0/60.0, 200)

	mux := http.NewServeMux()
	mux.Handle("/spectrogram/", http.StripPrefix("/spectrogram", ss.Handler()))

	const addr = "127.0.0.1:6060"
	log.Printf("spectroserver listening on http://%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func emitRamp(ss *spectro.SpectroServer, caller string, slope, wrap float64) {
	start := time.Now()
	for {
		now := time.Now()
		elapsed := now.Sub(start).Seconds()
		v := math.Mod(elapsed*slope, wrap)
		_ = ss.Emit(spectro.Observation{
			Time:       now,
			Dimensions: map[string]string{"caller": caller},
			Measures:   map[string]float64{"latency_ms": v},
		})
		time.Sleep(25 * time.Millisecond)
	}
}

func emitNoise(ss *spectro.SpectroServer, caller string, lo, hi float64) {
	for {
		_ = ss.Emit(spectro.Observation{
			Time:       time.Now(),
			Dimensions: map[string]string{"caller": caller},
			Measures:   map[string]float64{"latency_ms": lo + rand.Float64()*(hi-lo)},
		})
		time.Sleep(5 * time.Millisecond)
	}
}

// wave is one component of a synthetic multi-wave signal. Its amplitude
// and instantaneous frequency are each modulated by their own slow sine
// (ampSwing over ampDriftSecs; freqSwingHz over freqDriftSecs), so the
// signal breathes in amplitude and drifts in tempo without ever settling.
// phase is accumulated per tick using the instantaneous frequency so the
// modulation is true FM, not the visible artifact of dividing t by a
// time-varying period.
type wave struct {
	mean          float64
	baseAmp       float64
	ampSwing      float64
	ampDriftSecs  float64
	baseFreqHz    float64
	freqSwingHz   float64
	freqDriftSecs float64
	phase         float64
}

func emitMultiWave(ss *spectro.SpectroServer, caller string, waves []wave, jitter float64) {
	for i := range waves {
		waves[i].phase = rand.Float64() * 2 * math.Pi
	}
	start := time.Now()
	last := start
	for {
		now := time.Now()
		dt := now.Sub(last).Seconds()
		last = now
		elapsed := now.Sub(start).Seconds()

		v := rand.NormFloat64() * jitter
		for i := range waves {
			w := &waves[i]
			ampMod := math.Sin(2 * math.Pi * elapsed / w.ampDriftSecs)
			freqMod := math.Sin(2 * math.Pi * elapsed / w.freqDriftSecs)
			instAmp := w.baseAmp + w.ampSwing*ampMod
			instFreq := w.baseFreqHz + w.freqSwingHz*freqMod
			w.phase += 2 * math.Pi * instFreq * dt
			v += w.mean + instAmp*math.Sin(w.phase)
		}

		_ = ss.Emit(spectro.Observation{
			Time:       now,
			Dimensions: map[string]string{"caller": caller},
			Measures:   map[string]float64{"latency_ms": v},
		})
		time.Sleep(15 * time.Millisecond)
	}
}
