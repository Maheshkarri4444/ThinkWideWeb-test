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

---

## Part 4: Scale Memo (Scaling to 100 Million Events / Day)

### 1. What breaks first in the current implementation?
* **Memory Exhaustion (OOM Crash)**: Storing raw events and ever-expanding maps (`seenEvents map[string]struct{}`, unique contact sets, and slices) directly in process memory will rapidly consume tens of gigabytes of RAM. The Go runtime GC will freeze, and the OS OOM killer will terminate the process.
* **Global Lock Contention**: `sync.RWMutex` serializes every batch write across all campaigns. At ~1,200+ events/second sustained (peaking over 8,000 req/s), thread starvation and HTTP request timeouts (`504 Gateway Timeout`) will cause providers to aggressively retry, inducing a cascading collapse.
* **Lack of Durability**: Any pod restart or deployment permanently wipes out all aggregated stats.

---

### 2. What would you change, and in what order?
1. **Decouple Ingestion from Processing (Queue)**: Introduce a durable distributed log (Kafka or AWS Kinesis/SQS) immediately behind the HTTP endpoint. The API becomes a lightweight producer that pushes raw events and responds with `202 Accepted`.
2. **Move Deduplication to Distributed Cache**: Use an external fast-lookup store (Redis with TTL or a distributed Bloom filter) to check for duplicate `event_id`s over a rolling 48-hour window.
3. **Dedicated Stream Consumer Workers**: Horizontally scalable consumer pods pull from queue partitions and compute campaign aggregates asynchronously.
4. **Purpose-Built Storage Engine**: Persist raw immutable events in an OLAP database (ClickHouse) for ad-hoc analytical queries, and store pre-aggregated counters in Redis / PostgreSQL for sub-millisecond dashboard reads.

---

### 3. API Contract & Storage Design Changes
* **API Contract**:
  - `POST /events` changes from synchronous processing to asynchronous ingestion: returns `202 Accepted` with a batch tracking ID instead of waiting for persistence.
  - The payload structure remains backwards-compatible.
* **Storage Design**:
  - Shift from in-memory Go maps to **ClickHouse** (columnar storage optimal for high-throughput event logs) + **Redis** (for atomic counters and fast deduplication).

---

### 4. Introducing a Queue & New Challenges Created
* **Yes**, introducing Kafka/SQS is essential to absorb webhook traffic spikes and insulate backend storage from backpressure.
* **New Problems Introduced**:
  - **At-Least-Once Duplication**: Network retries between producer/broker or consumer rebalances will cause events to be read more than once.
  - **Out-of-Order Delivery**: Processing across multi-partition workers exacerbates out-of-order delivery.
  - **Eventual Consistency Lag**: Dashboards become slightly delayed (seconds to a minute) during heavy traffic surges.
  - **Operational Complexity**: Partition rebalancing, consumer lag monitoring, and Dead-Letter Queues (DLQ) for poisoned messages.

---

### 5. Where can duplicates sneak in that couldn't before?
* **Broker-to-Consumer Retries**: If a worker crashes after updating counters but before committing its Kafka offset, another worker re-processes the same events.
* **Distributed Race Conditions**: Two identical webhook retries arriving simultaneously at two separate API gateway pods can bypass non-atomic cache checks.

---

### 6. Keeping Counters Trustworthy
* **Idempotent Atomic Deduplication**: Use Redis atomic check-and-set (`SETNX event_id 1 EX 172800`) or database `ON CONFLICT (event_id) DO NOTHING`.
* **Approximate Uniques via HyperLogLog**: For unique opens/clicks across millions of users, use Redis `PFADD` (HyperLogLog) or Roaring Bitmaps. This computes unique reach with ~0.81% standard error while using only ~12KB per campaign regardless of audience size.
* **Nightly Reconciliation Batch Job**: Run an hourly/daily batch reconciliation (e.g., dbt/SQL over immutable event logs) to recalculate exact metrics and correct any streaming drift.

---

### 7. What would you monitor?
* **Ingress**: HTTP p95/p99 latency, 2xx vs. 4xx/5xx error rates, request throughput (req/sec).
* **Queue Health**: Consumer group lag (number of unconsumed messages) and dead-letter queue (DLQ) volume.
* **System Health**: Redis memory usage, ClickHouse ingestion rate, database CPU and disk I/O.
* **Business Health**: Provider retry rate, invalid/malformed event ratio.

---

### 8. What would you deliberately NOT solve yet?
* **Infinite Deduplication Window**: Do not attempt to store `event_id`s forever. Deduplicate across a rolling 48–72 hour window; provider retries outside that window are practically zero.
* **Complex In-Line Fraud / Bot Detection**: Don't burden the real-time ingestion pipeline with sophisticated click-fraud analysis. Run it asynchronously or in nightly analytics batches.
* **Multi-Region Active-Active Replication**: Keep the architecture single-region multi-AZ until business needs strictly require global active-active infrastructure.

---

## Part 5: The Angry Marketer

### Scenario Recap
* **10:00**: `Sent: 1,000,000 | Delivered: 970,000 | Opened: 250,000 | Clicked: 20,000`
* **10:30**: `Sent: 1,000,000 | Delivered: 975,000 | Opened: 248,000 | Clicked: 20,000`
* **Observation**: Delivered increased by +5,000; Opened dropped by -2,000.

---

### 1. Plausible Explanations Before Blaming Code
1. **Asynchronous Bot / Security Scanner Scrubbing**:
   Enterprise mail servers (e.g., Barracuda, Proofpoint, corporate firewalls) automatically pre-fetch and scan emails upon delivery. Relay or the delivery provider may run bot-detection algorithms that retroactively identified ~2,000 scanner opens and subtracted them to report genuine human engagement.
2. **Contact Deletions or GDPR / Unsubscribe Purges**:
   If the metric represents **unique opens** (distinct contacts reached) and 2,000 contacts unsubscribed or requested GDPR "Right to be Forgotten" deletion between 10:00 and 10:30, their records were scrubbed from campaign aggregates.
3. **Sliding Time-Window Dashboard Filter**:
   The marketer’s dashboard may have been toggled to a relative window (e.g., "Last 24 Hours"). As time moved forward 30 minutes, 2,000 opens that occurred 24.5 hours ago aged out of the active viewing window.
4. **Deduplication Reconciliation Job**:
   A scheduled background reconciliation job ran between 10:00 and 10:30, merged distributed session logs, identified 2,000 duplicate opens across cluster nodes, and corrected the count down.

---

### 2. What would you check first?
1. **Marketer's Dashboard Filters**: Confirm the active query parameters (e.g., all-time vs. rolling 24h / date-range filter).
2. **System Audit Logs & Background Jobs**: Check whether automated bot scrubbing, GDPR purge scripts, or periodic reconciliation tasks executed between 10:00 and 10:30.
3. **Recent Deployments or Incidents**: Check deployment logs for database cache flushes, worker restarts, or schema updates around 10:15.

---

### 3. How to decide whether this is a bug or expected behavior?
* Query the raw immutable event log directly:
  ```sql
  SELECT COUNT(DISTINCT contact_id) 
  FROM events 
  WHERE campaign_id = 'cmp_...' 
    AND type = 'opened' 
    AND is_bot = false;
  ```
* **Expected Behavior**: If the raw query confirms that genuine unique human opens are 248,000, then the 10:00 snapshot was unscrubbed/dirty data, and the 10:30 count is the corrected, accurate figure.
* **Bug**: If raw records demonstrate 250,000 distinct valid human opens, then an erroneous counter decrement, negative drift, or query caching bug occurred.

---

### 4. Is the `Delivered` number rising actually suspicious?
* **No, it is completely normal and expected.**
* Outbound message dispatching is asynchronous. High-volume email/SMS campaigns take minutes to hours to navigate external Mail Transfer Agents (MTAs), rate-limiting queues, and mobile carriers. Receiving 5,000 delivery receipts over a 30-minute span is standard real-world delivery behavior.

---

## Closing Summary

### What I Completed / What I Intentionally Skipped
* **Completed**:
  - Part 1: Strategic problem analysis, assumptions, and edge case scoping.
  - Part 2: High-performance Go HTTP service with per-item error isolation (`POST /events`), thread-safe in-memory store with `event_id` deduplication, Option B campaign stats API (`GET /stats`), and chronological event retrieval (`GET /events`).
  - Part 3: Diagnosed and fixed all 4 bugs in `debugging/` (cross-batch deduplication, cross-campaign `unique_opens`, UTC date formatting, and worker race conditions) verified against `expected_output.txt` and `go run -race`.
  - Part 4: Practical scale memo detailing queuing, distributed deduplication, and architectural trade-offs at 100M events/day.
  - Part 5: Systematic root cause analysis for the marketer's metric discrepancy.
* **Intentionally Skipped**:
  - Docker / Kubernetes manifests and cloud infrastructure setup (explicitly marked optional and unnecessary in the brief).
  - Web UI / frontend dashboards (zero extra credit; kept focus purely on resilient Go backend engineering).
  - Indefinite event storage retention (unnecessary for the scope of the assessment).

### If I Had Another Day
1. **Persistent SQLite / Embedded Badgedb Storage**: Replace the in-memory store with an embedded database with WAL mode for disk persistence without external infrastructure dependencies.
2. **Prometheus Metrics Endpoint (`GET /metrics`)**: Instrument HTTP request latencies, queue depths, duplicate rates, and rejected item counts.
3. **Rate Limiting & Webhook Authentication**: Implement HMAC signature verification (`X-Provider-Signature`) and token-bucket rate limiting to secure provider ingestion.
4. **Enhanced Paginated Query Filters**: Support filtering `GET /campaigns/{campaign_id}/events` by event type, date ranges, and cursor-based pagination.

