package camsim

import (
	_ "embed"
	"encoding/json"
	"io"
	"mime"
	"net/http"
)

//go:embed index.html
var page []byte

// Handler serves the standalone control page and its state/control API.
func (s *Simulator) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.State())
	})
	mux.HandleFunc("POST /api/control", func(w http.ResponseWriter, r *http.Request) {
		media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if media != "application/json" {
			http.Error(w, "请使用 application/json", http.StatusUnsupportedMediaType)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		var command struct {
			Action string `json:"action"`
		}
		if err := decoder.Decode(&command); err != nil {
			http.Error(w, "无效的控制请求："+err.Error(), http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "请求只能包含一个 JSON 对象", http.StatusBadRequest)
			return
		}
		switch command.Action {
		case "recognize":
			writeJSON(w, s.Recognize(r.Context()))
		case "report_current":
			state, err := s.ReportCurrent(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, state)
		case "retry":
			state, err := s.Retry(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, state)
		default:
			http.Error(w, "未知操作", http.StatusBadRequest)
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}
