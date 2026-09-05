package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	store *MemoryStore
}

func main() {
	s := &Server{
		store: NewMemoryStore(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", handlePing)
	mux.HandleFunc("POST /events", s.handlePostEvents)
	mux.HandleFunc("GET /campaigns/{campaignID}/stats", s.handleGetStats)
	mux.HandleFunc("GET /campaigns/{campaignID}/events", s.handleGetEvents)

	addr := ":8080"
	fmt.Printf("listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// handlePing handles health check requests.
func handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"message": "pong"})
}

// incomingEvent represents the raw structure of an event before validation.
type incomingEvent struct {
	EventID    string         `json:"event_id"`
	CampaignID string         `json:"campaign_id"`
	ContactID  string         `json:"contact_id"`
	Type       string         `json:"type"`
	Timestamp  string         `json:"timestamp"`
	Metadata   map[string]any `json:"metadata"`
}

// handlePostEvents ingests a JSON array of events with per-item error isolation.
func (s *Server) handlePostEvents(w http.ResponseWriter, r *http.Request) {
	// Limit request body size to 10MB to prevent denial of service.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body too large or unreadable"})
		return
	}

	// Decode into raw JSON messages to isolate malformed records.
	var rawItems []json.RawMessage
	if err := json.Unmarshal(body, &rawItems); err != nil {
		// Also support single event object for flexibility
		var singleItem json.RawMessage
		if errSingle := json.Unmarshal(body, &singleItem); errSingle == nil {
			rawItems = []json.RawMessage{singleItem}
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed JSON: expected array of events"})
			return
		}
	}

	var accepted, duplicates, rejected int

	for _, raw := range rawItems {
		var in incomingEvent
		if err := json.Unmarshal(raw, &in); err != nil {
			rejected++
			continue
		}

		// Field validation
		eventID := strings.TrimSpace(in.EventID)
		campaignID := strings.TrimSpace(in.CampaignID)
		contactID := strings.TrimSpace(in.ContactID)

		if eventID == "" || campaignID == "" || contactID == "" {
			rejected++
			continue
		}

		// Normalize type to lowercase
		eventType := strings.ToLower(strings.TrimSpace(in.Type))
		switch eventType {
		case "sent", "delivered", "opened", "clicked":
			// valid type
		default:
			// ignore or reject unknown event types (e.g. spam_report)
			rejected++
			continue
		}

		// Timestamp validation
		parsedTime, err := parseTimestamp(in.Timestamp)
		if err != nil {
			rejected++
			continue
		}

		event := Event{
			EventID:    eventID,
			CampaignID: campaignID,
			ContactID:  contactID,
			Type:       eventType,
			Timestamp:  parsedTime,
			Metadata:   in.Metadata,
		}

		if s.store.RecordEvent(event) {
			accepted++
		} else {
			duplicates++
		}
	}

	writeJSON(w, http.StatusOK, IngestionResult{
		Received:   len(rawItems),
		Accepted:   accepted,
		Duplicates: duplicates,
		Rejected:   rejected,
	})
}

// handleGetStats returns aggregated stats for one campaign (Option B format).
func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("campaignID")
	stats, found := s.store.GetStats(campaignID)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("campaign %q not found", campaignID),
		})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleGetEvents returns chronological events for a campaign.
func (s *Server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("campaignID")
	events, found := s.store.GetEvents(campaignID)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("campaign %q not found", campaignID),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"campaign_id": campaignID,
		"total":       len(events),
		"events":      events,
	})
}

// parseTimestamp handles standard RFC3339 and common date/time formats.
func parseTimestamp(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"02-01-2006 15:04",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp: %q", s)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}
