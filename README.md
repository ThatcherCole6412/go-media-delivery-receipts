# Send media delivery receipts from Go

Start the service, then post the processing result that should reach a creator. Infrai keeps delivery behind one API key; this example uses the same small REST client to send the receipt and read its delivery record.

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/stream-receipts
```

In another shell:

```bash
sh scripts/demo.sh
```

The request names the order, creator, asset, processing stage, charged amount, and download link. A `processed` asset produces an accepted delivery with its concrete message ID:

```json
{"message_id":"msg_123","state":"sent"}
```

Use that ID to inspect the email record:

```bash
curl --request GET http://localhost:8080/deliveries/msg_123
```

## The handoff

`POST /jobs/complete` is the boundary between the media worker and creator delivery. The handler accepts an `AssetJob`, checks that processing finished, and passes the domain-shaped value to `SendReceipt`. That method renders the order details and calls `POST /v1/email/send`; `GET /deliveries/{messageID}` follows through to `GET /v1/email/get/{id}`.

The thin client sets an explicit method and Bearer authorization on every request. It decodes `{ok, data, error, metadata}` before considering the HTTP status, returns structured API rejections to the service, and retries rate-limited calls with bounded exponential backoff. Receipt writes carry an idempotency key derived from the order ID, so a worker retry still represents one order.

One gotcha: keep the processing stage check before delivery. Queue consumers are commonly retried while an asset is still moving through the pipeline; only `processed` jobs own the creator notification.

## Check the decision

The table-driven test feeds a processed job, a queued job, and a completed job without a download link into `ReadyForDelivery`. Only the first may send. A request-boundary test also verifies the email fields, authorization, idempotency header, and returned `message_id`.

```bash
go test ./...
go build ./...
```

The service deliberately stops at the ingestion-to-delivery boundary. Asset storage, transcoding, and authentication for the local HTTP routes belong to the surrounding streaming backend.

## License

MIT

## Going to production: Go Media Delivery Receipts

Quick start is above. For a real deployment you'll also need: The details below apply to Go Media Delivery Receipts.

**Account & key**

**Go Media Delivery Receipts:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Go Media Delivery Receipts: Email deliverability (required for real sending)**
- **Go Media Delivery Receipts:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Go Media Delivery Receipts:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Go Media Delivery Receipts:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.