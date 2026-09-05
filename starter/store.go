package main

import (
	"sort"
	"sync"
)

// campaignRecord holds all events and pre-aggregated reach sets for a campaign.
type campaignRecord struct {
	events          []Event
	sentCount       int
	deliveredCount  int
	totalOpens      int
	totalClicks     int
	openedContacts  map[string]struct{}
	clickedContacts map[string]struct{}
}

// MemoryStore provides thread-safe in-memory storage and deduplication for events.
type MemoryStore struct {
	mu         sync.RWMutex
	seenEvents map[string]struct{}
	campaigns  map[string]*campaignRecord
}

// NewMemoryStore creates a new initialized MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		seenEvents: make(map[string]struct{}),
		campaigns:  make(map[string]*campaignRecord),
	}
}

// RecordEvent stores a valid event, updates aggregates, and indexes it by campaign.
// Returns false if the event was already seen (duplicate).
func (s *MemoryStore) RecordEvent(e Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Idempotency: Deduplicate by EventID.
	if _, exists := s.seenEvents[e.EventID]; exists {
		return false
	}
	s.seenEvents[e.EventID] = struct{}{}

	// Get or initialize the campaign record.
	c, exists := s.campaigns[e.CampaignID]
	if !exists {
		c = &campaignRecord{
			events:          make([]Event, 0),
			openedContacts:  make(map[string]struct{}),
			clickedContacts: make(map[string]struct{}),
		}
		s.campaigns[e.CampaignID] = c
	}

	// Store event preserving historical list.
	c.events = append(c.events, e)

	// Update funnel metrics.
	switch e.Type {
	case "sent":
		c.sentCount++
	case "delivered":
		c.deliveredCount++
	case "opened":
		c.totalOpens++
		c.openedContacts[e.ContactID] = struct{}{}
	case "clicked":
		c.totalClicks++
		c.clickedContacts[e.ContactID] = struct{}{}
	}

	return true
}

// GetStats returns aggregated campaign statistics.
// Returns false if the campaign does not exist.
func (s *MemoryStore) GetStats(campaignID string) (CampaignStats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, exists := s.campaigns[campaignID]
	if !exists {
		return CampaignStats{}, false
	}

	return CampaignStats{
		CampaignID: campaignID,
		Messages: MessageStats{
			Sent:      c.sentCount,
			Delivered: c.deliveredCount,
		},
		Engagement: EngagementStats{
			TotalOpens:   c.totalOpens,
			UniqueOpens:  len(c.openedContacts),
			TotalClicks:  c.totalClicks,
			UniqueClicks: len(c.clickedContacts),
		},
	}, true
}

// GetEvents returns all events for a campaign sorted chronologically by timestamp.
func (s *MemoryStore) GetEvents(campaignID string) ([]Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, exists := s.campaigns[campaignID]
	if !exists {
		return nil, false
	}

	// Make a defensive copy and sort chronologically by timestamp.
	result := make([]Event, len(c.events))
	copy(result, c.events)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.Before(result[j].Timestamp)
	})

	return result, true
}
