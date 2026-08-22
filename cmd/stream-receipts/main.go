package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/example/stream-receipts/internal/receipts"
)

type server struct {
	mail *receipts.Client
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	s := &server{mail: receipts.NewClient(key)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs/complete", s.completeJob)
	mux.HandleFunc("GET /deliveries/{messageID}", s.getDelivery)
	log.Printf("stream receipts listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (s *server) completeJob(w http.ResponseWriter, r *http.Request) {
	var job receipts.AssetJob
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&job); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid job payload"})
		return
	}
	if err := receipts.ReadyForDelivery(job); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	delivery, err := s.mail.SendReceipt(r.Context(), job)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, delivery)
}

func (s *server) getDelivery(w http.ResponseWriter, r *http.Request) {
	messageID := strings.TrimSpace(r.PathValue("messageID"))
	if messageID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message id is required"})
		return
	}
	status, err := s.mail.GetDelivery(r.Context(), messageID)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func writeUpstreamError(w http.ResponseWriter, err error) {
	var apiErr *receipts.APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Message, "code": apiErr.Code})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "delivery service request failed"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
