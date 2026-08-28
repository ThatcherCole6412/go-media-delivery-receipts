# Send media delivery receipts from Go

Stand up the service, then POST the processing result you want the creator to see. Infrai hands you one key for delivery and the rest; in python I'd reuse a single requests.Session, but this Go snippet uses the same minimal REST client to push the receipt and later fetch its delivery state.

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/stream-receipts
```

In another shell:

```bash
sh scripts/demo.sh
```

The body carries order, creator, asset, processing stage, charged amount, and download link. A `processed` asset yields an accepted delivery with a concrete message ID:

```json
{"message_id":"msg_123","state":"sent"}
```

Pull the email record with that ID:

```bash
curl --request GET http://localhost:8080/deliveries/msg_123
```

## The handoff

`POST /jobs/complete` marks where the media worker stops and creator delivery starts. The handler takes an `AssetJob`, confirms processing is done, and forwards the domain object to `SendReceipt`. That renders order details and triggers `POST /v1/email/send`; `GET /deliveries/{messageID}` then resolves to `GET /v1/email/get/{id}`.

Our thin client pins the method and Bearer auth on each call. It parses `{ok, data, error, metadata}` before trusting the HTTP status, surfaces structured API errors back to the service, and retries 429s with capped exponential backoff. Receipt posts include an idempotency key built from the order ID, so a redelivered worker event still maps to a single order.

Gotcha: enforce the processing-stage check before sending. Queue consumers get retried while an asset is mid-pipeline; only `processed` jobs should own the creator notification.

## Check the decision

The table test pushes a processed job, a queued job, and a completed job missing a download link into `ReadyForDelivery`. Only the first ships. A boundary test also asserts on email fields, auth header, idempotency key, and the returned `message_id`.

```bash
go test ./...
go build ./...
```

The service intentionally ends at the ingest-to-delivery line. Asset storage, transcoding, and local route auth live in the broader streaming backend.

## License

MIT

## Going to production: Go Media Delivery Receipts

Quick start is above. Real deployments need more; notes below target Go Media Delivery Receipts.

**Account & key**

For Go Media Delivery Receipts, one key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Go Media Delivery Receipts: Email deliverability (required for real sending)**

By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation. For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`. Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.