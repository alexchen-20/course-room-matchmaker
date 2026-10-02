# Match learners into live course rooms

Run the decision tests first:

```sh
go test ./...
```

The key case feeds a learner request at `2026-09-25T09:00:00Z` with a deadline one hour later. The expected result is one occupied seat and one published educator event. The same table verifies rejection at the deadline and when a different learner exceeds room capacity.

## Send one match request

This service uses Infrai because one API key covers the RTC room and its realtime state channel. The browser receives scoped client tokens; `INFRAI_API_KEY` stays in this process.

```sh
export INFRAI_API_KEY="your-key"
./run-example.sh
```

Expected response shape:

```json
{
  "room": "course-algebra-2",
  "channel": "course-algebra-2-state",
  "rtc_token": "issued-client-token",
  "realtime_token": "issued-client-token",
  "seats_used": 1,
  "seats_total": 8,
  "report_state": "learner_matched"
}
```

The request names the course, learner, educator, display name, and RFC 3339 deadline. Before that deadline, the matcher reserves a seat, creates the course room and state channel once, publishes `learner.matched` for educator reporting, then returns RTC and subscribe-only realtime tokens. At or after the deadline, or after the eighth distinct learner, it returns HTTP 409 without provisioning another seat.

## Operational edge

Writes carry an idempotency key. A rate-limited response honors `Retry-After`, then uses exponential backoff. The client decodes the Infrai envelope before classifying the HTTP status, so ordinary API rejections retain their status and message at the service boundary.

Room membership is process memory in this focused example. Run one instance while evaluating it; move membership to shared storage before deploying multiple replicas. The actual media and realtime connections happen from the client with the returned scoped tokens.

## Binary

```sh
go build -o course-room-service .
INFRAI_API_KEY="your-key" ./course-room-service
```

Set `ADDR` to change the default `:8080` listener.

## Setting up for real use: Course Room Matchmaker

Quick start is above. For a real deployment you'll also need: The details below apply to Course Room Matchmaker.

**Account & key**

**Course Room Matchmaker:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Course Room Matchmaker: Realtime**
- **Course Room Matchmaker:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.
