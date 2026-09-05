# Campaign Events — Engineering Notes

---

## Part 1: Read & Think First

### 1. Problem Interpretation
* **The Core Business**: Relay is an intermediary dispatching messages (email, SMS, push) via external delivery providers. When recipients interact with messages, providers notify Relay through webhook events (`sent`, `delivered`, `opened`, `clicked`).
* **The Technical Challenge**: Real-world provider webhooks are unreliable. They arrive out of order (e.g., `opened` before `delivered`), duplicate frequently due to network retries, contain malformed payloads, and spike concurrently.
* **The Goal**: Build a robust, thread-safe ingestion service in Go that withstands messy provider data, guarantees deduplication, and serves accurate campaign analytics for live marketing dashboards.

---

### 2. Assumptions
* **Idempotency & Uniqueness**: An event is uniquely identified by its `event_id`. Duplicate occurrences of the same `event_id` represent network retries and must be discarded without inflating metrics.
* **Partial Batch Acceptance**: A batch endpoint (`POST /events`) should not fail an entire request of 200 items because a single record is malformed. Valid events are accepted and recorded, while bad records are isolated and rejected.
* **Out-of-Order Lifecycle**: Events are processed independently. Because mobile networks and email clients behave unpredictably, an `opened` event arriving before `delivered` is valid and recorded immediately without artificial blocking.
* **Data Normalization**: Event types are case-insensitive (`OPENED` is treated as `opened`). Timestamps should adhere to RFC3339, but non-compliant timestamps should be caught gracefully rather than crashing the parser.
* **Storage Scope**: Given the current traffic (~few thousand events/day across ~50 campaigns), an in-memory thread-safe structure (using read/write mutexes) or embedded SQLite provides optimal performance and simplicity without distributed overhead.

---

### 3. Ambiguities Noticed in the Brief
* **"How many opened"**: Marketers care about both *total opens* (engagement volume) and *unique opens* (distinct recipients reached). The brief does not clarify which one the dashboard expects as the headline metric.
* **Event Identity Scope**: The payload specifies `event_id` is assigned by the provider. Real-world providers occasionally reuse IDs across different lifecycle stages (e.g., using message ID as event ID) or omit IDs. How strictly should conflicting payloads with identical `event_id`s be handled?
* **Batch Failure Contract**: The brief does not define HTTP status behavior for mixed batches: should it return `200 OK` (with an accepted/rejected count summary), `207 Multi-Status`, or `400 Bad Request`?
* **Campaign Lifecycle**: Campaigns are referenced dynamically by `campaign_id`. There is no pre-registration endpoint; campaigns are implicitly registered upon receiving their first event.

---

### 4. Questions for the Product Manager
1. **Metric Semantics**: Does the dashboard require *total event counts*, *unique contacts reached*, or both (e.g., `unique_opens` vs. `total_opens`)?
2. **Batch Failure Semantics**: If a provider sends 100 events where 98 are valid and 2 are corrupt, do you prefer accepting the 98 and reporting the 2 failures, or rejecting all 100 so the provider resends?
3. **Data Retention & Aggregation**: Do marketers need time-bucketed trends (e.g., hourly/daily breakdowns) or only lifetime aggregates per campaign?
4. **Freshness vs. Latency**: Is sub-second real-time consistency required for the dashboard, or is eventual consistency (a few seconds delay) acceptable as traffic expands?

---

### 5. Implementation Priorities & Order
1. **Resilient Ingestion (`POST /events`)**: Implement streaming/per-item JSON decoding that isolates corrupted records and prevents whole-batch failure.
2. **Deduplication & Concurrency-Safe Store**: Build thread-safe storage (`sync.RWMutex`) enforcing `event_id` idempotency and per-campaign indexing.
3. **Dashboard Stats API (`GET /campaigns/{campaign_id}/stats`)**: Expose clean, structured statistics (total counts + unique recipient metrics) with proper HTTP status codes (`404` for unknown campaigns).
4. **Seed Validation & Edge Case Hardening**: Validate against `starter/seed/events.json` (handling casing, invalid timestamps, missing fields, and duplicate IDs).
5. **Targeted Tests**: Write tests for the most fragile behaviors: duplicate event deduplication, concurrent ingestion safety, and malformed batch handling.
6. **(Stretch Goal)**: Implement paginated `GET /campaigns/{campaign_id}/events` only if time permits.
