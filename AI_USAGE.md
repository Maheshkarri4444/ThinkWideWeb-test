# AI Usage Disclosure (`AI_USAGE.md`)

---

### 1. Which tools you used
* **Google Antigravity**: Primary AI assistant utilized throughout this engineering assessment.
* *Note*: I usually use Claude for my day-to-day development, but as my subscription had expired, I used Antigravity for this task.

---

### 2. Roughly what you used them for
* **Task Decomposition & Data Analysis**: Breaking down the PM brief into plain English and inspecting real-world edge cases in `starter/seed/events.json` (uncovering missing fields, invalid timestamps, casing discrepancies, and conflicting duplicate IDs).
* **Incremental Implementation**: Scaffolding and testing HTTP handlers (`GET /ping`, `POST /events`, and `GET /campaigns/{campaign_id}/stats`).
* **Concurrency & Debugging Analysis**: Analyzing race conditions and logic flaws in `debugging/main.go` under `go run -race`.
* **Documentation Structuring**: Formulating structured markdown reports for `NOTES.md`, `BUGS.md`, and `README.md`.

---

### 3. One suggestion from AI that you rejected or changed, and why
* **Rejected Suggestion**: The AI offered to write all the backend service logic and endpoints in one single, massive code drop.
* **Why**: I prefer following a disciplined, step-by-step engineering procedure rather than dumping a whole solution at once. I rejected the all-at-once approach and directed the workflow incrementally:
  1. Testing environment setup with a simple `/ping` health probe first.
  2. Discussing and finalizing the `/stats` response schema (choosing Option B) based on dashboard needs.
  3. Reviewing and confirming the ingestion edge-case strategy before writing any store or handler code.

---

### 4. One thing AI helped you understand
* It briefed the assignment requirements and underlying business context exceptionally well in simple, actionable terms.
* In addition, it highlighted the practical flaw of standard Go JSON array decoding (`[]Event`), illustrating how one malformed record can abort an entire 200-event batch and why per-item decoding via `[]json.RawMessage` is essential for webhook resilience.

---

### 5. Anything AI generated that you then had to debug
* As the task was straightforward and we broke it down cleanly, the AI didn't really make mistakes that needed debugging.
* However, based on my experience using AI, AI often hallucinates when given heavy tasks all at once. That is why we must deal with the task step by step and verify each part along the way.
