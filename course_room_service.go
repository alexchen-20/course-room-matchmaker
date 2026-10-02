package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	matcher := NewMatchmaker(8, time.Now, NewInfraiClient(key))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /match", matchHandler(matcher))
	addr := envOr("ADDR", ":8080")
	log.Printf("course room matcher listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func matchHandler(matcher *Matchmaker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input MatchRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
			return
		}
		result, err := matcher.Match(r.Context(), input)
		if err != nil {
			status := http.StatusBadGateway
			switch {
			case errors.Is(err, ErrDeadlinePassed), errors.Is(err, ErrRoomFull):
				status = http.StatusConflict
			default:
				var apiErr *APIError
				if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
					status = apiErr.HTTPStatus
				}
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
