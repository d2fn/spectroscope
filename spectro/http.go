package spectro

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// Handler returns an http.Handler serving spectrogram queries for this server.
// Mount it under whatever prefix you want from your application's mux:
//
//	mux := http.NewServeMux()
//	mux.Handle("/spectrogram/", http.StripPrefix("/spectrogram", ss.Handler()))
//
// GET /<metric> returns the current spectrogram for that metric as JSON.
// Optional ?yBins=N query parameter overrides the default y-axis bin count.
func (ss *SpectroServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{metric}", ss.handleSpectrogram)
	return mux
}

func (ss *SpectroServer) handleSpectrogram(w http.ResponseWriter, r *http.Request) {
	metric := r.PathValue("metric")
	yBins := 0
	if s := r.URL.Query().Get("yBins"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			yBins = n
		}
	}

	spec, err := ss.Query(SpectrogramQuery{Measure: metric, YBins: yBins})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SpectrogramReply{Spectrogram: *spec})
}
