# EventForge webhooks

EventForge publishes QueueFlow task lifecycle events to tenant-scoped HTTP
endpoints. Postgres is the source of truth for endpoints, encrypted signing
secrets, deliveries, attempts, and delivery results. A separate webhook worker
claims due deliveries and sends them over HTTP.

All `/v1/webhooks` routes require the organization's Bearer API key:

```http
Authorization: Bearer queueflow-dev-key
```

## Supported events

- `task.created`
- `task.processing`
- `task.completed`
- `task.failed`
- `task.dead_letter`
- `task.cancelled`

Each endpoint receives only its selected event types while it is active.
Endpoints and delivery logs are scoped to the authenticated organization.

## Manage endpoints

Production endpoint URLs must use HTTPS. Localhost HTTP URLs are accepted only
when the API runs with `ALLOW_INSECURE_LOCAL_WEBHOOKS=true`.

Create an endpoint:

```bash
curl -sS http://localhost:8080/v1/webhooks/endpoints \
  -X POST \
  -H 'Authorization: Bearer queueflow-dev-key' \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "billing events",
    "url": "https://example.com/webhooks/flowforge",
    "event_types": ["task.completed", "task.failed"],
    "is_active": true
  }'
```

The `201 Created` response includes a generated `secret`:

```json
{
  "id": "0ca3de76-6055-47eb-9485-c94d3791755e",
  "org_id": "00000000-0000-4000-8000-000000000001",
  "name": "billing events",
  "url": "https://example.com/webhooks/flowforge",
  "event_types": ["task.completed", "task.failed"],
  "is_active": true,
  "secret": "generated-base64url-signing-secret"
}
```

Save the secret immediately. It is returned only by create and rotate
operations; get and list responses never expose it.

List, get, update, and soft-delete an endpoint:

```bash
curl -sS -H 'Authorization: Bearer queueflow-dev-key' \
  http://localhost:8080/v1/webhooks/endpoints

curl -sS -H 'Authorization: Bearer queueflow-dev-key' \
  http://localhost:8080/v1/webhooks/endpoints/ENDPOINT_ID

curl -sS http://localhost:8080/v1/webhooks/endpoints/ENDPOINT_ID \
  -X PATCH \
  -H 'Authorization: Bearer queueflow-dev-key' \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "billing lifecycle",
    "event_types": ["task.completed", "task.failed", "task.dead_letter"],
    "is_active": true
  }'

curl -i http://localhost:8080/v1/webhooks/endpoints/ENDPOINT_ID \
  -X DELETE \
  -H 'Authorization: Bearer queueflow-dev-key'
```

Deleting is a soft delete and also deactivates the endpoint. To pause delivery
without deleting it, patch `is_active` to `false`.

## Rotate a signing secret

```bash
curl -sS http://localhost:8080/v1/webhooks/endpoints/ENDPOINT_ID/rotate-secret \
  -X POST \
  -H 'Authorization: Bearer queueflow-dev-key'
```

The response contains the new secret once:

```json
{"secret":"new-generated-base64url-signing-secret"}
```

Rotation takes effect immediately. Update the receiver before processing new
deliveries, and retain the returned value securely because it cannot be
retrieved later.

## Verify signatures

Every delivery is an HTTP `POST` with `Content-Type: application/json` and:

| Header | Value |
| --- | --- |
| `X-FlowForge-Event` | Event type, such as `task.completed` |
| `X-FlowForge-Delivery` | Delivery UUID |
| `X-FlowForge-Timestamp` | Unix timestamp in seconds |
| `X-FlowForge-Signature` | `v1=` followed by the hex HMAC-SHA256 digest |

The signed bytes are:

```text
timestamp + "." + raw JSON payload
```

Verify the signature against the raw request body before parsing JSON. Do not
re-serialize the payload because byte changes invalidate the digest.

```go
package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func Verify(secret, timestamp string, rawBody []byte, signature string) bool {
	if !strings.HasPrefix(signature, "v1=") {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(rawBody)

	expected := "v1=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
```

Also parse `X-FlowForge-Timestamp` and reject timestamps outside a short
tolerance (for example, five minutes) to reduce replay risk. Store recently
processed `X-FlowForge-Delivery` values if the receiver must reject duplicate
deliveries. EventForge delivery is at least once, so receivers must be
idempotent.

## Delivery and retry lifecycle

1. A matching task lifecycle event creates a `pending` delivery in Postgres.
2. The webhook worker claims it as `delivering` and increments
   `attempt_count`.
3. Any `2xx` response marks it `delivered`.
4. A network error, timeout, redirect, or non-`2xx` response marks it
   `retrying` and schedules `next_attempt_at` with exponential backoff.
5. Reaching `WEBHOOK_DELIVERY_MAX_ATTEMPTS` marks it `failed`.
6. A manual retry resets a `failed` or `retrying` delivery to `pending` with
   zero attempts.

Response bodies are retained for diagnostics and truncated to 4 KiB. Delivery
ordering is not guaranteed across retries.

## Inspect and retry deliveries

List delivery logs, optionally filtered by endpoint, status, or event type:

```bash
curl -sS -G http://localhost:8080/v1/webhooks/deliveries \
  -H 'Authorization: Bearer queueflow-dev-key' \
  --data-urlencode 'endpoint_id=ENDPOINT_ID' \
  --data-urlencode 'status=failed' \
  --data-urlencode 'event_type=task.failed' \
  --data-urlencode 'limit=50'
```

Inspect one delivery, including its payload, response, and last error:

```bash
curl -sS -H 'Authorization: Bearer queueflow-dev-key' \
  http://localhost:8080/v1/webhooks/deliveries/DELIVERY_ID
```

Retry a failed or retrying delivery:

```bash
curl -sS http://localhost:8080/v1/webhooks/deliveries/DELIVERY_ID/retry \
  -X POST \
  -H 'Authorization: Bearer queueflow-dev-key'
```

List results use opaque cursor pagination. An organization cannot list or read
another organization's endpoints or deliveries.

## Run the webhook worker

Docker Compose starts the API, QueueFlow workers, and EventForge worker:

```bash
make up
```

To run only the webhook worker against existing Postgres:

```bash
make webhook-worker
```

Webhook worker and API configuration:

| Environment variable | Default | Used by |
| --- | ---: | --- |
| `DB_DSN` | local Postgres DSN | API and webhook worker |
| `WEBHOOK_SECRET_ENCRYPTION_KEY` | local development value | API and webhook worker |
| `ALLOW_INSECURE_LOCAL_WEBHOOKS` | `false` | API |
| `WEBHOOK_DELIVERY_TIMEOUT_SEC` | `10` | webhook worker |
| `WEBHOOK_DELIVERY_POLL_INTERVAL_SEC` | `1` | webhook worker |
| `WEBHOOK_DELIVERY_BATCH_SIZE` | `50` | webhook worker |
| `WEBHOOK_DELIVERY_MAX_ATTEMPTS` | `5` | API event publisher |
| `WEBHOOK_DELIVERY_INITIAL_BACKOFF_SEC` | `5` | webhook worker |
| `WEBHOOK_DELIVERY_MAX_BACKOFF_SEC` | `3600` | webhook worker |

The API and every webhook worker must use the same
`WEBHOOK_SECRET_ENCRYPTION_KEY`.

## Dashboard

The server-rendered dashboard keeps the FlowForge API key out of browser
JavaScript. EventForge pages are:

- `/webhooks`
- `/webhooks/new`
- `/webhooks/{id}`
- `/webhook-deliveries`
- `/webhook-deliveries/{id}`

Create and rotate screens display a new signing secret once. Copy it before
leaving the page.

## Production security

- Set a strong, stable `WEBHOOK_SECRET_ENCRYPTION_KEY` in a secret manager.
  Losing or changing it prevents existing signing secrets from being decrypted.
- Never log API keys, webhook signing secrets, encryption keys, or full
  signature base strings.
- Use HTTPS endpoints and keep `ALLOW_INSECURE_LOCAL_WEBHOOKS=false`.
- Rotate a webhook secret immediately if it may be compromised.
- Verify signatures with constant-time comparison and enforce timestamp
  freshness.
- Make receivers idempotent by delivery UUID.
- Restrict delivery-log access because payloads and response bodies may contain
  application data.

## Troubleshooting

- `401 unauthorized`: verify the Bearer API key exists and is not revoked.
- `404 webhook_endpoint_not_found` or `webhook_delivery_not_found`: verify the
  ID and that the API key belongs to the same organization.
- `400 validation_failed` when creating an HTTP endpoint: use HTTPS, or enable
  insecure local webhooks only for a localhost target.
- No delivery created: confirm the endpoint is active and subscribes to the
  emitted event type.
- Delivery remains `pending`: start `cmd/webhook-worker` and verify its
  `DB_DSN`.
- Delivery is `retrying`: inspect `response_status`, `last_error`, and
  `next_attempt_at`; check receiver availability and response time.
- Signature verification fails: use the raw request bytes, the timestamp header,
  the current endpoint secret, and the exact `timestamp + "." + payload`
  format.
- Decryption errors after deployment: restore the stable
  `WEBHOOK_SECRET_ENCRYPTION_KEY` used when endpoints were created, or rotate
  every endpoint secret using the new stable key.
- Local HTTP receiver rejected: set `ALLOW_INSECURE_LOCAL_WEBHOOKS=true` on the
  API and use `localhost`, `127.0.0.1`, or `::1`.
