package api

import (
	"log/slog"
	"net/http"
	"time"

	"icecoreverdict/internal/application"
)

type Server struct {
	app    *application.Service
	logger *slog.Logger
	mux    *http.ServeMux
}

func New(app *application.Service, logger *slog.Logger) *Server {
	s := &Server{app: app, logger: logger, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return s.logging(s.mux) }
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.Health)
	s.mux.HandleFunc("POST /api/v1/cases", s.CreateCase)
	s.mux.HandleFunc("POST /api/v1/cases/{case_id}/commands", s.SubmitCommand)
	s.mux.HandleFunc("GET /api/v1/cases/{case_id}", s.GetCase)
	s.mux.HandleFunc("GET /api/v1/cases/{case_id}/events", s.GetEvents)
	s.mux.HandleFunc("GET /api/v1/cases/{case_id}/archive", s.DownloadArchive)
	s.mux.HandleFunc("POST /api/v1/cases/{case_id}/archive/verify", s.VerifyArchive)
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	n, e := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, e
}
func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		s.logger.Info("http_request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "bytes", sw.bytes, "duration_ms", time.Since(start).Milliseconds())
	})
}
