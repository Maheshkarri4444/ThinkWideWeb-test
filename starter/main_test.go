package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestIngestAndStats(t *testing.T) {
	s := &Server{store: NewMemoryStore()}

	// Batch with valid events, duplicates, case variations, and malformed items.
	payload := `[
		{"event_id": "evt_1", "campaign_id": "cmp_1", "contact_id": "ct_1", "type": "sent", "timestamp": "2026-08-10T10:00:00Z"},
		{"event_id": "evt_2", "campaign_id": "cmp_1", "contact_id": "ct_1", "type": "delivered", "timestamp": "2026-08-10T10:01:00Z"},
		{"event_id": "evt_3", "campaign_id": "cmp_1", "contact_id": "ct_1", "type": "OPENED", "timestamp": "2026-08-10T10:05:00Z"},
		{"event_id": "evt_4", "campaign_id": "cmp_1", "contact_id": "ct_1", "type": "opened", "timestamp": "2026-08-10T10:15:00Z"},
		{"event_id": "evt_1", "campaign_id": "cmp_1", "contact_id": "ct_1", "type": "sent", "timestamp": "2026-08-10T10:00:00Z"},
		{"event_id": "", "campaign_id": "cmp_1", "contact_id": "ct_1", "type": "sent", "timestamp": "2026-08-10T10:00:00Z"},
		{"event_id": "evt_bad", "campaign_id": "cmp_1", "contact_id": "ct_1", "type": "opened", "timestamp": "not-a-time"}
	]`

	req := httptest.NewRequest("POST", "/events", bytes.NewBufferString(payload))
	w := httptest.NewRecorder()
	s.handlePostEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res IngestionResult
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Received != 7 {
		t.Errorf("expected 7 received, got %d", res.Received)
	}
	if res.Accepted != 4 {
		t.Errorf("expected 4 accepted, got %d", res.Accepted)
	}
	if res.Duplicates != 1 {
		t.Errorf("expected 1 duplicate, got %d", res.Duplicates)
	}
	if res.Rejected != 2 {
		t.Errorf("expected 2 rejected, got %d", res.Rejected)
	}

	// Verify stats for cmp_1
	reqStats := httptest.NewRequest("GET", "/campaigns/cmp_1/stats", nil)
	reqStats.SetPathValue("campaignID", "cmp_1")
	wStats := httptest.NewRecorder()
	s.handleGetStats(wStats, reqStats)

	if wStats.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", wStats.Code)
	}

	var stats CampaignStats
	if err := json.NewDecoder(wStats.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats: %v", err)
	}

	if stats.Messages.Sent != 1 {
		t.Errorf("expected 1 sent, got %d", stats.Messages.Sent)
	}
	if stats.Messages.Delivered != 1 {
		t.Errorf("expected 1 delivered, got %d", stats.Messages.Delivered)
	}
	if stats.Engagement.TotalOpens != 2 {
		t.Errorf("expected 2 total opens, got %d", stats.Engagement.TotalOpens)
	}
	if stats.Engagement.UniqueOpens != 1 {
		t.Errorf("expected 1 unique open (same contact), got %d", stats.Engagement.UniqueOpens)
	}
}

func TestSeedEventsSurvives(t *testing.T) {
	data, err := os.ReadFile("seed/events.json")
	if err != nil {
		t.Fatalf("failed to read seed/events.json: %v", err)
	}

	s := &Server{store: NewMemoryStore()}
	req := httptest.NewRequest("POST", "/events", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	s.handlePostEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res IngestionResult
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Received != 235 {
		t.Errorf("expected 235 received, got %d", res.Received)
	}
	if res.Accepted == 0 {
		t.Errorf("expected accepted > 0, got %d", res.Accepted)
	}

	// Test campaign stats endpoint
	reqStats := httptest.NewRequest("GET", "/campaigns/cmp_summer_sale/stats", nil)
	reqStats.SetPathValue("campaignID", "cmp_summer_sale")
	wStats := httptest.NewRecorder()
	s.handleGetStats(wStats, reqStats)

	if wStats.Code != http.StatusOK {
		t.Fatalf("expected 200 for cmp_summer_sale, got %d", wStats.Code)
	}
}
