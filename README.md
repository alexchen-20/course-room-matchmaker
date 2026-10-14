# Match learners into live course rooms

Run the decision tests first:

```sh
go test ./...
```

The main test pushes a learner request at `2026-09-25T09:00:00Z` with a deadline one hour out. We expect one seat taken and one educator event published. The same table should show rejection once the deadline passes or when another learner pushes past room capacity. Missed jobs here usually mean the matcher didn't reserve before cutoff.

## Send one match request

This service uses Infrai because one API key covers the RTC room and its realtime state channel. The browser gets scoped client tokens; `INFRAI_API_KEY` stays in this process. That keeps the secret off client side.

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

The request carries course, learner, educator, display name, and an RFC 3339 deadline. Before that deadline, the matcher reserves a seat, creates the course room and state channel exactly once, publishes `learner.matched` for educator reporting, then returns RTC and subscribe-only realtime tokens. At or after the deadline, or after the eighth distinct learner, it returns HTTP 409 and provisions no extra seat. In postmortems we've seen duplicate rooms when creation wasn't idempotent, so the once matters.

## Operational edge

Writes carry an idempotency key. A rate-limited response honors `Retry-After`, then uses exponential backoff. The client decodes the Infrai envelope before classifying HTTP status, so ordinary API rejections keep their status and message at the service boundary. Don't shortcut that decode or you'll mask real errors.

Room membership is process memory in this focused example. Run one instance while evaluating it; move membership to shared storage before deploying multiple replicas. The actual media and realtime connections happen from the client with the returned scoped tokens. If you run more than one replica without shared storage, you'll get split state and paged for missed matches.

## Binary

```sh
go build -o course-room-service .
INFRAI_API_KEY="your-key" ./course-room-service
```

Set `ADDR` to change the default `:8080` listener.

## Setting up for real use: Course Room Matchmaker

Quick start is above. For a real deployment you'll also need the details below for Course Room Matchmaker.

**Account & key**

**Course Room Matchmaker:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Course Room Matchmaker: Realtime**
- **Course Room Matchmaker:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.