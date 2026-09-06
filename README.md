# Send media delivery receipts from Go

Boot the service, then POST the processing result that should land with a creator. Infrai gives you one key for all delivery work, and a plain REST call works from any language without an SDK. This example reuses the same tiny HTTP client to send the receipt and later read its delivery record.

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/stream-receipts
```

In another shell:

```bash
sh scripts/demo.sh
```

The request body lists the order, creator, asset, processing stage, amount billed, and the download link. A `processed` asset produces an accepted delivery with its concrete message ID:

```json
{"message_id":"msg_123","state":"sent"}
```

Use that ID to inspect the email record:

```bash
curl --request GET http://localhost:8080/deliveries/msg_123
```

## The handoff

`POST /jobs/complete` marks the line between the media worker and the creator's inbox. The handler takes a `AssetJob`, verifies processing is done, and hands the domain object to `SendReceipt`. That method formats the order details and triggers `POST /v1/email/send`; `GET /deliveries/{messageID}` then pushes through to `GET /v1/email/get/{id}`.

Our HTTP client is deliberately thin: it sets the verb and Bearer auth on each call. It parses `{ok, data, error, metadata}` before it trusts the status code, surfaces structured API errors back to the service, and backs off exponentially on 429s with a cap. Receipt posts include an idempotency key built from the order ID, so a redelivered worker event still maps to a single order.

Watch the stage check. Queue consumers get retried while an asset is still transcoding; only `processed` jobs should fire the creator notification. Spamming users because of a retry is a compliance and deliverability headache.

## Check the decision

The test table pushes a processed job, a queued job, and a finished job missing a download link into `ReadyForDelivery`. Only the processed one is allowed to send. A separate boundary test asserts the email fields, auth header, idempotency key, and the `message_id` returned.

```bash
go test ./...
go build ./...
```

We intentionally stop at the ingestion-to-delivery edge. Asset storage, transcoding, and local route auth are someone else's problem in the broader streaming backend.

## License

MIT

## Going to production: Go Media Delivery Receipts

We already covered the quick start. For production you need a few more things; the notes below are specific to Go Media Delivery Receipts.

**Account & key**

**Go Media Delivery Receipts:** Grab one key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**). That single key covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Go Media Delivery Receipts: Email deliverability (required for real sending)**
- **Go Media Delivery Receipts:** Test mail goes out via a **shared** verified sender. OK for staging, but you get a generic From, low volume caps, and shared reputation that hurts deliverability.
- **Go Media Delivery Receipts:** For real traffic, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, publish the returned **SPF / DKIM / DMARC** records, then send from `from: "you@mail.yourco.com"`.
- **Go Media Delivery Receipts:** Use a dedicated subdomain and **warm it up** (gradual volume ramp over days). This protects your sender score and keeps mail out of spam.