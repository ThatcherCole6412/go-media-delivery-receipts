#!/bin/sh
set -eu

curl --fail-with-body --request POST http://localhost:8080/jobs/complete \
  --header 'Content-Type: application/json' \
  --data '{"order_id":"ord-42","creator_email":"creator@example.com","asset_name":"launch-trailer.mp4","stage":"processed","amount_cents":2400,"download_url":"https://media.example/download/ord-42"}'
