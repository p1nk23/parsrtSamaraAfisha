package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"checker-parser-service/internal/api"
	"checker-parser-service/internal/parser"
	"checker-parser-service/internal/repository"
	"checker-parser-service/internal/storage"
)

func main() {
	addr := env("HTTP_ADDR", ":8081")
	dataPath := env("DATA_PATH", "data/events.json")
	databaseURL := env("DATABASE_URL", "")
	browserTimeout := durationEnv("BROWSER_TIMEOUT", 45*time.Second)
	repo, storageMode, closeRepo := mustCreateRepository(databaseURL, dataPath)
	defer closeRepo()
	yp := parser.YandexParser{
		UseBrowser:     boolEnv("USE_BROWSER", true),
		BrowserTimeout: browserTimeout,
		BrowserBin:     env("BROWSER_BIN", ""),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"service":     "checker-parser-service",
			"useBrowser":  yp.UseBrowser,
			"storageMode": storageMode,
			"dataPath":    dataPath,
			"defaultURL":  parser.DefaultURL,
			"browserBin":  yp.BrowserBin,
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
		})
	})
	readEventsHandler := func(w http.ResponseWriter, r *http.Request) {
		events, err := repo.LoadEvents()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				events = sampleEvents()
			} else {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, events)
	}
	mux.HandleFunc("GET /event", readEventsHandler)
	mux.HandleFunc("GET /events", readEventsHandler)
	mux.HandleFunc("POST /parse/yandex-afisha", func(w http.ResponseWriter, r *http.Request) {
		handleParse(w, r, yp, repo)
	})
	mux.HandleFunc("GET /parse/yandex-afisha", func(w http.ResponseWriter, r *http.Request) {
		handleParse(w, r, yp, repo)
	})

	log.Printf("checker parser-service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, recoverer(cors(mux))))
}

func handleParse(w http.ResponseWriter, r *http.Request, yp parser.YandexParser, repo repository.EventRepository) {
	url := r.URL.Query().Get("url")
	ctx, cancel := context.WithTimeout(r.Context(), yp.BrowserTimeout+15*time.Second)
	defer cancel()
	result, err := yp.ParseDetailed(ctx, url)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	if err := repo.SaveEvents(result.Events); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func mustCreateRepository(databaseURL string, dataPath string) (repository.EventRepository, string, func()) {
	if databaseURL == "" {
		return storage.NewFileEventRepository(dataPath), "file", func() {}
	}
	repo, err := storage.NewPostgresEventRepository(databaseURL)
	if err != nil {
		log.Fatalf("postgres init failed: %v", err)
	}
	return repo, "postgres", func() { _ = repo.Close() }
}

func sampleEvents() []api.Event {
	now := time.Now().UTC().Format(time.RFC3339)
	currency, ticketURL, service := "RUB", parser.DefaultURL, "Яндекс Афиша"
	min := 0
	return []api.Event{{
		ID:                   1,
		Title:                "Пример импортированного события",
		ShortDescription:     "Это демо-запись parser-service. Запусти POST /parse/yandex-afisha, чтобы заменить ее реальными данными.",
		FullDescription:      "Пример показывает формат данных, который уже совместим с фронтендом: ApiEventFull и eventSessions.",
		Category:             "other",
		OrganizerName:        "Яндекс Афиша",
		OrganizerDescription: "Импортировано из Яндекс Афиши",
		Status:               "published",
		CreatedAt:            now,
		UpdatedAt:            now,
		EventSessions: []api.EventSession{{
			ID:                1,
			StartAt:           now,
			IsOnline:          false,
			TicketMinPrice:    &min,
			Currency:          &currency,
			TicketURL:         &ticketURL,
			TicketServiceName: &service,
			Status:            "published",
			CreatedAt:         now,
			UpdatedAt:         now,
			Venue:             &api.Venue{Name: "Самара", Address: "Самара"},
		}},
	}}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func boolEnv(k string, d bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	if v == "" {
		return d
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
func durationEnv(k string, d time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	if parsed, err := time.ParseDuration(v); err == nil {
		return parsed
	}
	if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	return d
}
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				writeErr(w, http.StatusInternalServerError, fmt.Errorf("internal parser error: %v", rec))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
