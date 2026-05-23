package spectro

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strconv"
)

//go:embed ui.html
var uiHTML []byte

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
	mux.HandleFunc("GET /ui", ss.handleUI)
	mux.HandleFunc("GET /_metrics", ss.handleMetrics)
	mux.HandleFunc("GET /{metric}", ss.handleSpectrogram)
	return mux
}

func (ss *SpectroServer) handleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(uiHTML)
}

func (ss *SpectroServer) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Metrics []string `json:"metrics"`
	}{Metrics: ss.measures.names})
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
