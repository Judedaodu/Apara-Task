# Apara — architecture and operations

Context for the stack: how services fit together, where files live, how configuration is loaded, and how to run and verify everything with Docker or native toolchains.

---

## Part 1 — Context

### What problem this repository simulates

The task models **liquidity and settlement orchestration** between banks: the platform **does not hold customer funds**; it **routes instructions**. A full production stack would include a settlement layer (e.g. Corda); **this repository does not run Corda**. It provides:

- A **Go** service that models the **edge**: the only HTTP entry point for the “external” `POST /initialize` call.
- A **Kotlin / Spring Boot** service that models the **core**: settlement rules, liquidity pools, duplicate detection, and simulated failure modes.

The corridor is **Akin → Pawpaw**. External networks are **simulated** with environment variables (e.g. AkinNet failure before the core is contacted).

### Architectural pattern in one sentence

**Clients talk only to Go. Go talks to Kotlin. Kotlin owns the simulated business state.** Configuration flows from `.env` → Docker Compose → each container’s environment.

---

## Part 2 — High-level architecture

### Logical view

```mermaid
flowchart LR
  subgraph external [External caller]
    Client[HTTP client or curl]
  end

  subgraph host [Your machine]
    DC[docker compose]
    subgraph net [Docker network apara-net]
      Go[go-api :8080]
      Kt[kotlin-core :8081]
    end
  end

  Client -->|"POST /initialize"| Go
  Go -->|"POST /core/receive-submit + HMAC header"| Kt
  Go -.->|"GET /health"| Go
  Client -.->|"GET /actuator/health"| Kt
```

- **go-api** is the **ingress** for the scripted client contract (`/initialize`).
- **kotlin-core** is **not** intended as a public internet-facing API in this exercise; it is what the edge calls internally. You still expose `8081` on the host for **health checks**, **debugging**, and **direct curl tests** of core behaviour (for example, ghost payments).

### Request lifecycle (happy path)

1. Caller sends **`POST http://localhost:8080/initialize`** with JSON body containing `templateId`, `amount`, `currency`, `receiverAccount`, `senderBank`.
2. **go-api** generates a **fingerprint** (unique id for this instruction attempt).
3. **go-api** runs the **AkinNet simulator**: if it “fails,” the caller receives **HTTP 422** and **`AKINNET_FAILURE`** — Kotlin is never contacted.
4. If AkinNet succeeds, **go-api** serializes a **`/core/receive-submit`** body (`fingerprint`, `templateId`, `amount`, `currency`, `senderBank`), computes **HMAC-SHA256** using `SPONSOR_HMAC_SECRET`, and POSTs to **`KOTLIN_CORE_URL`** (inside Docker this is `http://kotlin-core:8081`).
5. **kotlin-core** verifies the HMAC against the **raw bytes** of that JSON, then runs **SettlementService** rules (pools, duplicates, optional Cordapp failure, optional delayed scenario).
6. If Kotlin returns **HTTP 200**, **go-api** responds to the client with **`state: "IN_TRANSIT"`** and the fingerprint, per the fixed client contract.

### Why Docker Compose sits at the root

**Docker Compose** at the repo root starts **both** services on **`apara-net`** with DNS **`kotlin-core`** / **`go-api`**, **health-ordered startup** (Go after Kotlin is healthy), and **`.env`** injected into containers.

---

## Part 3 — Repository layout

```
Apara/                              ← project root; run Compose from here
├── docker-compose.yml              ← orchestrates services, network, health, ports
├── .env.example                    ← documented defaults (copy to .env)
├── .env                            ← your local secrets/overrides (gitignored)
├── README.md                       ← quick reference, curl recipes
├── PROJECT_GUIDE.md                ← this document
├── .gitignore
├── .gitattributes                  ← line endings for gradlew (Linux CI)
│
├── .github/workflows/ci.yml        ← PR pipeline: Go, Kotlin, then Docker smoke
│
├── go-api/                         ← Go edge service
│   ├── go.mod                      ← Go module definition (Go 1.21)
│   ├── main.go                     ← HTTP server: /health, /initialize, Kotlin client
│   ├── main_test.go                ← unit tests for handlers
│   ├── Dockerfile                  ← multi-stage build: compile → Alpine runtime
│   ├── .dockerignore               ← limits build context
│   └── .golangci.yml               ← linter config used in CI
│
└── kotlin-core/                    ← Spring Boot core service
    ├── settings.gradle.kts         ← Gradle project name
    ├── build.gradle.kts            ← Spring Boot, Kotlin JVM, dependencies
    ├── gradle.properties
    ├── gradlew / gradlew.bat       ← Gradle wrapper (reproducible builds)
    ├── gradle/wrapper/             ← wrapper jar + properties (Gradle 8.7)
    ├── Dockerfile                  ← JDK stage builds bootJar; JRE stage runs jar
    ├── .dockerignore
    └── src/
        ├── main/
        │   ├── kotlin/com/apara/core/
        │   │   ├── KotlinCoreApplication.kt   ← Spring Boot entrypoint
        │   │   ├── CoreController.kt            ← POST /core/receive-submit
        │   │   ├── CachedBodyFilter.kt          ← buffers body for HMAC verify
        │   │   ├── SettlementService.kt         ← pools, ghost, failures
        │   │   └── SettlementModels.kt          ← request/response data classes
        │   └── resources/application.yml        ← port 8081, actuator exposure
        └── test/kotlin/.../SettlementServiceTest.kt
```

### How data flows through Kotlin files

| File | Role |
|------|------|
| `KotlinCoreApplication.kt` | Bootstraps Spring; component scan lives under `com.apara.core`. |
| `CachedBodyFilter.kt` | For `POST /core/receive-submit` only, reads the body once, stores bytes on the request, and replays them to Jackson so the **same bytes** used for JSON parsing are the **same bytes** used for HMAC verification. |
| `CoreController.kt` | HTTP mapping for `/core/receive-submit`; calls `SettlementService`; maps domain outcomes to HTTP status codes (for example, conflict vs bad gateway). |
| `SettlementService.kt` | **Domain logic**: sponsor signature check delegation, idempotency / ghost detection, pool arithmetic, simulated Cordapp and sponsor-delay behaviour. |
| `SettlementModels.kt` | DTOs aligned with the JSON contract. |
| `application.yml` | Server port **8081**; Actuator exposes **health** (required for Compose healthchecks and CI). |

### How Go files fit together

| File | Role |
|------|------|
| `main.go` | Single binary: registers routes, reads environment variables, implements AkinNet simulation, calls Kotlin over HTTP, applies HMAC to outbound body. |
| `main_test.go` | Tests health JSON shape, AkinNet 422 path, unreachable core, and happy path with a mock Kotlin server. |

There is **no shared code** between Go and Kotlin: integration is **HTTP + JSON + `SPONSOR_HMAC_SECRET`**.

---

## Part 4 — Configuration

### Environment variables

These names are **fixed by the task specification**. They are listed and commented in **`.env.example`**.

| Variable | Role |
|----------|------|
| `SPONSOR_HMAC_SECRET` | Shared secret. Go signs the outbound JSON body; Kotlin verifies. |
| `KOTLIN_CORE_URL` | Base URL for Kotlin (in Compose, overridden to `http://kotlin-core:8081` for `go-api`). |
| `AKN_POOL_INITIAL` | Starting balance for the simulated AKN pool (minor units). |
| `PAW_POOL_INITIAL` | Starting balance for the simulated PAW pool. |
| `AKINNET_FAILURE_RATE` | Probability in `[0,1]` that Go fails before calling Kotlin (HTTP 422). |
| `SPONSOR_DELAY_SECONDS` | Used by Kotlin to decide when the **DELAYED** demo path applies (see README / code). |
| `CORDAPP_FAILURE_RATE` | Probability that Kotlin simulates a Corda/Cordapp failure (HTTP 502 from core). |

### Where values are loaded

1. You copy **`.env.example` → `.env`** at the repository root.
2. **`docker-compose.yml`** references `env_file: .env` for **both** services.
3. **`docker-compose.yml`** additionally sets `KOTLIN_CORE_URL` for **`go-api`** via `environment:` so that inside the bridge network the hostname **`kotlin-core`** resolves correctly. Your `.env` may still contain `KOTLIN_CORE_URL` for documentation consistency; Compose’s `environment` entry wins for that service when both are present (Docker merges with explicit env taking precedence for `go-api`).

If you run services **without** Compose, you must set `KOTLIN_CORE_URL=http://localhost:8081` manually for Go.

---

## Part 5 — Prerequisites

**Option A — Docker (default)**

Install **Docker Desktop** (Windows or macOS) or **Docker Engine + Compose plugin** (Linux). Both images build the apps inside Docker; you do **not** need Go or a JDK on the host for `docker compose up`.

Verify:

```bash
docker --version
docker compose version
```

**Option B — Native toolchains (optional)**

For local iteration without rebuilding images.

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.21+ | `cd go-api && go run .` or `go test ./...` |
| JDK | 17 | Gradle and Spring Boot 3 |
| Kotlin | via Gradle (1.9.x in `build.gradle.kts`) | You do not install Kotlin separately if you use Gradle |

On Windows, install Go and JDK from official installers and ensure **both are on `PATH`**. Point `JAVA_HOME` to JDK 17 for Gradle.

---

## Part 6 — Run the stack with Docker (PowerShell)

Use the **repository root** (the directory that contains `docker-compose.yml`).

### Step 1 — Confirm directory

```powershell
cd <path-to-cloned-repo>
dir docker-compose.yml
```

### Step 2 — Environment file

Copy the template to **`.env`** (not committed):

```powershell
copy .env.example .env
```

Open `.env` in an editor if you want to tune failure rates later. Defaults are safe for a first run.

### Step 3 — Docker engine

Start **Docker Desktop** (or your Docker daemon). `docker compose` needs the engine running.

### Step 4 — Build and start containers

```powershell
docker compose up --build
```

What happens:

1. Compose builds **`kotlin-core`** image from `kotlin-core/Dockerfile` (Gradle `bootJar` in the build stage, JRE in the runtime stage).
2. Compose builds **`go-api`** image from `go-api/Dockerfile` (static Go binary on Alpine).
3. Compose attaches both to **`apara-net`**.
4. **`kotlin-core`** starts first. Its **healthcheck** runs `curl` against **`/actuator/health`** until it returns success.
5. Only after Kotlin is healthy does **`go-api`** start (because of `depends_on: condition: service_healthy`).
6. **go-api** healthcheck runs `wget` against **`/health`**.

Leave this terminal open to watch logs, or add `-d` for detached mode:

```powershell
docker compose up --build -d
docker compose logs -f
```

### Step 5 — Verify health endpoints

In a **second** terminal:

```powershell
curl.exe http://localhost:8080/health
curl.exe http://localhost:8081/actuator/health
```

On Windows PowerShell, prefer **`curl.exe`** so the request is not handled by `Invoke-WebRequest`.

You should see HTTP 200 responses. Go returns JSON including `"status":"ok"` and `"service":"go-api"`. Kotlin returns Actuator JSON with `"status":"UP"` at the top level.

### Step 6 — Happy path (`/initialize`)

```powershell
curl.exe -X POST http://localhost:8080/initialize `
  -H "Content-Type: application/json" `
  -d "{\"templateId\":\"demo-1\",\"amount\":100,\"currency\":\"AKN\",\"receiverAccount\":\"ACC-001\",\"senderBank\":\"BANK-AKIN\"}"
```

Expected:

- HTTP **200**
- Response body includes **`fingerprint`**, **`state":"IN_TRANSIT"`**, and **`timestamp`**

That indicates a successful path through Go to Kotlin and the expected client response shape.

### Step 7 — Shut down cleanly

```powershell
docker compose down
```

Add `-v` if you want to remove named volumes (this project uses mostly default volumes; `down` removes containers and the default network).

---

## Part 7 — Step-by-step: run services without Docker (optional)

Use this when debugging locally.

### 7.1 Start Kotlin first

```powershell
cd kotlin-core
.\gradlew.bat bootRun
```

Wait until the log shows Tomcat listening on **8081**.

### 7.2 Start Go with a local Kotlin URL

In another terminal:

```powershell
cd <path-to-cloned-repo>\go-api
$env:KOTLIN_CORE_URL="http://localhost:8081"
$env:SPONSOR_HMAC_SECRET="dev-secret-change-in-prod"
go run .
```

### 7.3 Call the same endpoints as in Part 6

If `go` is not installed, install Go 1.21+ or skip this path and use Docker only.

---

## Part 8 — Continuous integration

**`.github/workflows/ci.yml`** runs on **pull requests to `main`**:

1. **Job `go`:** `go mod tidy`, `golangci-lint`, `go test ./...`, `go build`.
2. **Job `kotlin`:** JDK 17, `./gradlew build test`.
3. **Job `integration`:** runs after both succeed; copies `.env.example` to `.env`, runs `docker compose up -d --build`, waits for both health URLs, curls `/initialize`, then `docker compose down -v`.

If Compose works locally but CI fails, check **`gradlew` line endings** (`.gitattributes`), **Dockerfile** paths, and **ports 8080/8081**.

---

## Part 9 — Troubleshooting

| Symptom | Likely cause | What to do |
|--------|----------------|------------|
| `docker compose` cannot connect to daemon | Docker Desktop not running | Start Docker; retry. |
| `go-api` keeps restarting | Kotlin never becomes healthy | `docker compose logs kotlin-core` — wait for Spring; check JVM startup errors. |
| `initialize` returns **401** from core path | HMAC mismatch | Ensure **same** `SPONSOR_HMAC_SECRET` in `.env` for both services; for direct Kotlin calls, sign the **exact** JSON bytes. |
| `initialize` returns **502** | Kotlin simulated Cordapp failure | Set `CORDAPP_FAILURE_RATE=0` in `.env` or retry (if non-zero, it is probabilistic). |
| Port **8080** or **8081** in use | Another process on host | Stop that process or change the left side of `ports:` in `docker-compose.yml` (and use the new port in curl). |
