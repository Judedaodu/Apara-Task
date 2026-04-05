# Apara — local stack (Go + Kotlin)

Minimal **go-api** (edge) and **kotlin-core** (settlement simulation) wired for the Apara take-home: one corridor (Akin → Pawpaw), external rails mocked, shared contracts from the brief.

For a **senior-level walkthrough** of architecture, how every folder fits together, prerequisites, and a full local runbook, read **`PROJECT_GUIDE.md`**.

## Prerequisites

- **Go** 1.21+
- **Kotlin** 1.9+ with **JDK** 17+ (only needed if you build `kotlin-core` outside Docker)
- **Docker Desktop** (Compose v2)

## Quick start (under five minutes)

1. Clone the repository.
2. Copy environment defaults and adjust if needed:

   ```bash
   cp .env.example .env
   ```

3. Start everything:

   ```bash
   docker compose up --build
   ```

4. Wait until both services report **healthy** in `docker compose ps` (Compose waits on `kotlin-core` before starting `go-api`).

You should see `go-api` on port **8080** and `kotlin-core` on **8081**.

## Service map

| Service       | Port | Role |
|---------------|------|------|
| **go-api**    | 8080 | Edge API: AkinNet simulation, forwards signed payloads to Kotlin. |
| **kotlin-core** | 8081 | Spring Boot: pools, duplicate detection (“ghost” payments), Cordapp failure simulation. |

### Endpoints

| Method | Path | Service | Description |
|--------|------|---------|-------------|
| `GET`  | `/health` | go-api | JSON: `{ "status": "ok", "service": "go-api" }` |
| `POST` | `/initialize` | go-api | Client submit; on success returns `state: "IN_TRANSIT"`. |
| `GET`  | `/actuator/health` | kotlin-core | Spring Actuator: `{ "status": "UP" }` (plus standard Actuator fields if enabled). |
| `POST` | `/core/receive-submit` | kotlin-core | Internal: settlement step (HMAC-signed body from go-api). |

Docker network: **`apara-net`**. Containers reach each other by DNS name (`kotlin-core`, `go-api`).

## Environment variables

All variables are documented in **`.env.example`** (copy to **`.env`**). Important ones:

- **`KOTLIN_CORE_URL`** — In Compose, `go-api` is set to `http://kotlin-core:8081` (service DNS).
- **`SPONSOR_HMAC_SECRET`** — Shared HMAC-SHA256 secret; go-api signs the raw JSON body, kotlin-core verifies `X-Sponsor-Signature` (hex).
- **`AKINNET_FAILURE_RATE`** — `0..1` probability that go-api returns **422** `AKINNET_FAILURE` before calling Kotlin.
- **`CORDAPP_FAILURE_RATE`** — `0..1` probability that kotlin-core simulates a Corda/Cordapp failure (**502**).
- **`SPONSOR_DELAY_SECONDS`** — When `> 0`, kotlin-core may return **`DELAYED`** for demo amounts (see below).
- **`AKN_POOL_INITIAL` / `PAW_POOL_INITIAL`** — Starting simulated pool balances (minor units).

## Test scenarios (curl)

Replace host/ports if you are not using default mapping.

### Happy path (client → go-api)

```bash
curl -sS -X POST http://localhost:8080/initialize \
  -H "Content-Type: application/json" \
  -d '{"templateId":"t1","amount":100,"currency":"AKN","receiverAccount":"recv-1","senderBank":"bank-akin"}'
```

Expect HTTP **200** and `state` **`IN_TRANSIT`**.

### AkinNet failure (HTTP 422)

Set in `.env`:

```env
AKINNET_FAILURE_RATE=1
```

Restart the stack (`docker compose up -d --build`), then run the same `/initialize` request. Expect **422** with `error: "AKINNET_FAILURE"`.

Set `AKINNET_FAILURE_RATE` back to `0` for normal operation.

### Ghost payment (duplicate fingerprint)

Kotlin rejects a second submit with the same `fingerprint` (double-spend). go-api always generates a new fingerprint, so exercise this **against kotlin-core** with a stable body and valid HMAC.

With the default secret from `.env.example`:

```bash
BODY='{"fingerprint":"ghost-demo","templateId":"t1","amount":50,"currency":"AKN","senderBank":"bank-akin"}'
SIG=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac "dev-secret-change-in-prod" | sed 's/^.* //')

curl -sS -X POST http://localhost:8081/core/receive-submit \
  -H "Content-Type: application/json" \
  -H "X-Sponsor-Signature: $SIG" \
  -d "$BODY"

curl -sS -X POST http://localhost:8081/core/receive-submit \
  -H "Content-Type: application/json" \
  -H "X-Sponsor-Signature: $SIG" \
  -d "$BODY"
```

The second call should return **`GHOST_DETECTED`** (HTTP **409**).

### Pool rejection (debit would go negative)

Use an **amount larger than `AKN_POOL_INITIAL`** on a fresh stack (see `.env`), e.g.:

```bash
curl -sS -X POST http://localhost:8080/initialize \
  -H "Content-Type: application/json" \
  -d "{\"templateId\":\"t2\",\"amount\":20000000,\"currency\":\"AKN\",\"receiverAccount\":\"recv-2\",\"senderBank\":\"bank-akin\"}"
```

Expect kotlin-core to respond with **`POOL_REJECTED`** (surfaced as **409** through go-api). Pool balances stay consistent (no partial debit).

### DELAYED sponsor path

Ensure **`SPONSOR_DELAY_SECONDS`** is **greater than 0** in `.env` (default `60`). Submit an amount divisible by **1_000_000** (e.g. `1000000`):

```bash
curl -sS -X POST http://localhost:8080/initialize \
  -H "Content-Type: application/json" \
  -d '{"templateId":"t-delay","amount":1000000,"currency":"AKN","receiverAccount":"recv-3","senderBank":"bank-akin"}'
```

kotlin-core returns **`DELAYED`** for that pattern (no long sleep; suitable for CI and local demos). The PDF also mentions tuning **`SPONSOR_DELAY_SECONDS`** when exploring timing behaviour (e.g. raising it for manual experiments).

### Cordapp failure (simulated)

Set `CORDAPP_FAILURE_RATE=1` in `.env`, restart, and call `/initialize` until kotlin-core returns **502** (probabilistic). Set back to `0` afterwards.

## Troubleshooting

1. **`docker compose up` hangs or go-api never starts**  
   **Cause:** `kotlin-core` health check failing (Kotlin/JVM still starting, or port conflict).  
   **Fix:** Run `docker compose logs -f kotlin-core`, wait for Spring to finish startup; ensure port **8081** is free on the host. Increase `start_period` in `docker-compose.yml` if your machine is slow.

2. **`/initialize` returns 401 / invalid signature**  
   **Cause:** `SPONSOR_HMAC_SECRET` mismatch between go-api and kotlin-core, or manual curls to kotlin-core without a correct `X-Sponsor-Signature`.  
   **Fix:** Use the same secret in `.env` for both services; for direct kotlin-core calls, compute the HMAC over the **exact** JSON bytes (see ghost example).

3. **`connection refused` to localhost ports**  
   **Cause:** Containers not running, or wrong host (e.g. calling from another machine without port publishing).  
   **Fix:** `docker compose ps`; confirm ports `8080:8080` and `8081:8081`; restart with `docker compose up -d`.

## Development (without Docker)

- **go-api:** `cd go-api && go run .`
- **kotlin-core:** `cd kotlin-core && ./gradlew bootRun` (Unix) or `gradlew.bat bootRun` (Windows)

Point `KOTLIN_CORE_URL` at `http://localhost:8081` when running go-api locally.

## Evidence for submission

After `docker compose up`, capture logs or `docker compose ps` showing **both services healthy** (screenshot or terminal output), as requested in the brief.

## CI

GitHub Actions (on pull requests to `main`): Go lint + tests + build, Gradle build + tests, then Docker Compose smoke (health + sample `/initialize`).
