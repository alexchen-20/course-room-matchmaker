#!/bin/sh
set -eu

: "${INFRAI_API_KEY:?set INFRAI_API_KEY}"

go run . &
service_pid=$!
trap 'kill "$service_pid" 2>/dev/null || true' EXIT
sleep 1

curl --fail-with-body -X POST http://localhost:8080/match \
  -H 'Content-Type: application/json' \
  -d '{"course_id":"algebra-2","learner_id":"learner-42","display_name":"Mina","educator_id":"educator-7","deadline":"2027-06-30T16:00:00Z"}'
