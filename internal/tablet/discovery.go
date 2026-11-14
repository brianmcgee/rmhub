package tablet

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// maxDiscard is the maximum number of bytes that the server reads from
// a body that it discards. For a larger body, the server closes the
// connection after the response.
const maxDiscard = 1 << 20

// Discovery serves the discovery endpoints, the service locator and the
// telemetry endpoints.
type Discovery struct {
	// Host is the announced host: tablet.announced_host.
	Host string
}

// Register adds the endpoints to the mux. requireUser checks the user
// token of the sync report endpoint.
func (d *Discovery) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	mux.HandleFunc("GET /discovery/v1/endpoints", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"notifications": d.Host, "webapp": d.Host})
	})
	mux.HandleFunc("GET /discovery/v1/webapp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, hostStatus{Host: d.Host, Status: "OK"})
	})
	mux.HandleFunc("GET /service/json/1/{service}", func(w http.ResponseWriter, r *http.Request) {
		host := d.Host
		if r.PathValue("service") == "blob-storage" {
			host = "https://" + host
		}
		writeJSON(w, http.StatusOK, hostStatus{Host: host, Status: "OK"})
	})

	mux.HandleFunc("POST /analytics/v2/events", func(w http.ResponseWriter, r *http.Request) {
		discard(w, r)
		writeJSON(w, http.StatusCreated, map[string]string{"message": "Success"})
	})
	for _, p := range []string{"POST /v1/reports", "POST /v2/reports", "POST /report/v1", "POST /v2/events", "POST /settings/v1/beta"} {
		mux.HandleFunc(p, discardOK)
	}
	mux.Handle("POST /sync/reports/v1", requireUser(http.HandlerFunc(discardOK)))
	mux.HandleFunc("GET /settings/v1/beta", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"enrolled": false, "available": true})
	})
}

type hostStatus struct {
	Host   string `json:"Host"`
	Status string `json:"Status"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// discard reads at most maxDiscard bytes of the body. Past the limit,
// http.MaxBytesReader makes the server close the connection after the
// response.
func discard(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, maxDiscard))
}

func discardOK(w http.ResponseWriter, r *http.Request) {
	discard(w, r)
	w.WriteHeader(http.StatusOK)
}

// NotFound answers each request with status 404. It logs the method and
// the path at the warning level, but not the query, the body or the
// headers.
func NotFound(log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Warn("unknown tablet request", "method", r.Method, "path", r.URL.Path)
		http.NotFound(w, r)
	})
}
