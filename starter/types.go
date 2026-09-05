package main

import "time"

// Event represents a single lifecycle action reported by a message delivery provider.
type Event struct {
	// EventID is the unique identifier assigned by the provider (used for deduplication).
	EventID string `json:"event_id"`

	// CampaignID identifies which marketing campaign this event belongs to.
	CampaignID string `json:"campaign_id"`

	// ContactID identifies the recipient contact (used to calculate unique opens/clicks).
	ContactID string `json:"contact_id"`

	// Type is the event lifecycle stage (e.g. sent, delivered, opened, clicked).
	Type string `json:"type"`

	// Timestamp is when the event occurred at the provider in UTC.
	Timestamp time.Time `json:"timestamp"`

	// Metadata holds optional arbitrary key-value pairs (e.g. clicked URL).
	Metadata map[string]any `json:"metadata,omitempty"`
}

// IngestionResult provides an audit breakdown of how an event batch was processed.
type IngestionResult struct {
	Received   int `json:"received"`   // Total events submitted in request
	Accepted   int `json:"accepted"`   // Valid new events stored
	Duplicates int `json:"duplicates"` // Idempotent retries safely ignored
	Rejected   int `json:"rejected"`   // Malformed items discarded
}

// MessageStats tracks outbound delivery funnel metrics.
type MessageStats struct {
	// Sent is the total number of messages successfully dispatched.
	Sent int `json:"sent"`

	// Delivered is the total number of messages confirmed delivered to recipient devices.
	Delivered int `json:"delivered"`
}

// EngagementStats tracks recipient interaction and unique reach metrics.
type EngagementStats struct {
	// TotalOpens is the gross number of times messages were opened.
	TotalOpens int `json:"total_opens"`

	// UniqueOpens is the number of distinct contacts who opened at least once.
	UniqueOpens int `json:"unique_opens"`

	// TotalClicks is the gross number of link clicks recorded.
	TotalClicks int `json:"total_clicks"`

	// UniqueClicks is the number of distinct contacts who clicked at least once.
	UniqueClicks int `json:"unique_clicks"`
}

// CampaignStats is the response payload for GET /campaigns/{campaignID}/stats (Option B).
type CampaignStats struct {
	// CampaignID identifies the campaign for which statistics are aggregated.
	CampaignID string `json:"campaign_id"`

	// Messages contains the delivery funnel counts.
	Messages MessageStats `json:"messages"`

	// Engagement contains open and click interaction counts.
	Engagement EngagementStats `json:"engagement"`
}
