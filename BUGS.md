# Debugging Exercise — Bug Report (`BUGS.md`)

This report documents the four bugs identified and resolved in `debugging/main.go`. Each fix adhered to the minimal-change requirement without restructuring the program.

---

### Bug 1: Incomplete Deduplication Across Batches

* **1. What the bug is:**
  Events with the same `event_id` appearing in different 200-event batches were being counted multiple times instead of being deduplicated across the entire file.
* **2. Why it happens:**
  `seen := make(map[string]bool)` was initialized as a local variable inside `processBatch()`. Each time a new batch arrived, a brand new `seen` map was instantiated, discarding previously tracked `event_id`s.
* **3. The fix:**
  Moved the `seen` map declaration to the package level (`var seen = map[string]bool{}`) so deduplication state persists across all batches.
* **4. How verified:**
  Compared total counts against `expected_output.txt`. For instance, `cmp_A sent` dropped from an inflated `2025` down to the expected `1987`.

---

### Bug 2: Cross-Campaign Contact Collision in `unique_opens`

* **1. What the bug is:**
  `unique_opens` undercounted contacts who opened emails across multiple campaigns. A contact opening in Campaign A was counted, but their subsequent open in Campaign B was ignored.
* **2. Why it happens:**
  The tracking map `openedBy` was keyed solely on `ev.ContactID` (`openedBy[ev.ContactID]`). Once a contact opened any email, their contact ID was marked as `true` globally, preventing them from being counted in any other campaign.
* **3. The fix:**
  Composite key used for tracking: `ev.CampaignID + ":" + ev.ContactID`. A contact's open is now tracked independently per campaign.
* **4. How verified:**
  `cmp_A unique_opens` corrected from `549` to `942`, matching `expected_output.txt` exactly.

---

### Bug 3: Timezone Distortion in Daily Delivery Buckets

* **1. What the bug is:**
  Daily `delivered` events were landing in the wrong calendar date bucket, and an extra ghost bucket (`2026-08-08`) appeared on local machines ahead of UTC.
* **2. Why it happens:**
  The date formatting logic used `ev.Timestamp.Local().Format("2006-01-02")`. Converting UTC timestamps to the machine's local timezone shifted late-evening events into the following day.
* **3. The fix:**
  Replaced `.Local()` with `.UTC()`: `ev.Timestamp.UTC().Format("2006-01-02")`.
* **4. How verified:**
  Verified that `delivered` bucket dates span strictly from `2026-08-01` to `2026-08-07`, and daily bucket counts match `expected_output.txt` across all campaigns.

---

### Bug 4: Concurrency Data Race in Counter Updates

* **1. What the bug is:**
  A data race condition occurred during parallel worker processing, causing non-deterministic counts and failing Go's race detector.
* **2. Why it happens:**
  8 parallel worker goroutines received events from the `jobs` channel and executed `apply(ev)` simultaneously. Within `apply()`, shared integer fields (`cs.Sent++`, `cs.Delivered++`, etc.) were mutated concurrently without synchronization.
* **3. The fix:**
  Introduced a package-level mutex `applyMu sync.Mutex` inside `apply()` (`applyMu.Lock()` and `defer applyMu.Unlock()`) to synchronize concurrent pointer mutations.
* **4. How verified:**
  Executed `go run -race . events.jsonl`. Go's race detector reported 0 data races, and run-to-run output remained 100% deterministic.

---

## Verification Output

```bash
maheshkarri@Maheshs-MacBook-Air debugging % go run . events.jsonl > actual.txt
maheshkarri@Maheshs-MacBook-Air debugging % diff actual.txt expected_output.txt
maheshkarri@Maheshs-MacBook-Air debugging % go run -race . events.jsonl
campaign=cmp_A sent=1987 delivered=1914 opened=1283 clicked=550 unique_opens=942
  2026-08-01 delivered=270
  2026-08-02 delivered=277
  2026-08-03 delivered=275
  2026-08-04 delivered=268
  2026-08-05 delivered=280
  2026-08-06 delivered=278
  2026-08-07 delivered=266
campaign=cmp_B sent=1695 delivered=1542 opened=1115 clicked=467 unique_opens=833
  2026-08-01 delivered=221
  2026-08-02 delivered=210
  2026-08-03 delivered=218
  2026-08-04 delivered=202
  2026-08-05 delivered=210
  2026-08-06 delivered=236
  2026-08-07 delivered=245
... (all campaigns match byte-for-byte with 0 race warnings)
```
