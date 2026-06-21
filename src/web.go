package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed web/dist
var webAssets embed.FS

type ServeOptions struct {
	DBPath  string
	Address string
}

// ServeIndex serves the embedded Vue app and the read-only file index API.
func ServeIndex(opts ServeOptions) error {
	server := &http.Server{
		Addr:              opts.Address,
		Handler:           newHTTPRouter(opts.DBPath),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Printf("Serving index UI at http://%s\n", opts.Address)
	return server.ListenAndServe()
}

func newHTTPRouter(dbPath string) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	router.Route("/api", func(api chi.Router) {
		api.Get("/files", listFilesHandler(dbPath))
		api.Get("/health", healthHandler)
	})
	router.Handle("/*", spaHandler())

	return router
}

func listFilesHandler(dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Open the database per request so the SPA can be left running without
		// holding a long-lived read lock that blocks future scans.
		store, err := OpenIndexStore(dbPath, true)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer store.Close()

		files, err := store.ListFiles()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"count": len(files),
			"files": files,
		})
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		fmt.Fprintf(w, `{"error":%q}`, err.Error())
	}
}

// spaHandler serves real assets when present and falls back to index.html for
// client-side routes such as /files or /duplicates.
func spaHandler() http.Handler {
	dist, err := fs.Sub(webAssets, "web/dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "embedded SPA assets are unavailable", http.StatusInternalServerError)
		})
	}

	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if cleanPath == "." || cleanPath == "" {
			serveIndexHTML(w, dist)
			return
		}
		if assetExists(dist, cleanPath) {
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndexHTML(w, dist)
	})
}

func assetExists(dist fs.FS, name string) bool {
	file, err := dist.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()

	info, err := file.Stat()
	return err == nil && !info.IsDir()
}

func serveIndexHTML(w http.ResponseWriter, dist fs.FS) {
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.Error(w, "index.html is missing from embedded SPA assets", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(index)
}
