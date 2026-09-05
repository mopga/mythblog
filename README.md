# MythBlog

Self-hosted publishing engine behind Oddity Archive: one Go binary, SQLite, server-rendered templates, an admin UI, an agent-friendly API, and optional S3-compatible object storage.

## Current status

Milestones 1–6 complete:

- environment configuration;
- structured JSON logging;
- graceful shutdown;
- chi router;
- SQLite with WAL, foreign keys and 5s busy timeout;
- embedded ordered migrations;
- `GET /health`.
- transactional articles/categories/sources/relations model;
- Bearer authentication with independently rotatable admin and Hermes keys;
- agent-friendly article filters by `slug` and `title`;
- machine-readable validation/conflict/not-found errors;
- article/category REST CRUD.
- server-rendered home, article and category pages;
- safe Markdown rendering (raw HTML disabled);
- public pages expose only `published` articles.
- built-in `/admin` UI with Russian form login, signed session cookie and CSRF protection;
- manual article create/edit/delete, Markdown, categories, sources and relations.
- object-storage abstraction with local development driver and Cloudflare R2;
- media upload with size/type validation, decode, resize and safe JPEG output;
- article cover/media metadata and public rendering.
- Hermes API contract documented in `docs/hermes.md`.

## Project structure

```text
cmd/oddity              service entry point
internal/config         environment configuration
internal/database       SQLite connection and migrations
internal/content        domain validation and repository
internal/storage        local/R2 object storage
internal/server         HTTP API, SSR pages and admin UI
docs/hermes.md          agent integration contract
```

## Linux/systemd example

```ini
[Unit]
Description=Oddity Archive
After=network.target

[Service]
DynamicUser=yes
StateDirectory=oddity-archive
WorkingDirectory=/opt/oddity-archive
Environment=ODDITY_ADDR=127.0.0.1:8890
EnvironmentFile=/etc/oddity-archive.env
Environment=ODDITY_DB_PATH=/var/lib/oddity-archive/oddity.db
Environment=STORAGE_DRIVER=local
Environment=LOCAL_STORAGE_PATH=/var/lib/oddity-archive/media
Environment=PUBLIC_BASE_URL=https://oddity.example.com
ExecStart=/opt/oddity-archive/oddity
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Use an `EnvironmentFile` with `0600` permissions in real production rather than
embedding keys in a unit.

> Note: the MVP initially writes JPEG for web-safe output because the environment has
> no production-grade pure-Go WebP encoder. The storage/media interface does not
> depend on this choice.

## Media API

```text
POST   /api/v1/media              Bearer admin or Hermes
DELETE /api/v1/media/{id}         Bearer admin only
```

Storage configuration:

```text
STORAGE_DRIVER=local|r2
LOCAL_STORAGE_PATH
PUBLIC_BASE_URL
R2_ENDPOINT
R2_BUCKET
R2_ACCESS_KEY_ID
R2_SECRET_ACCESS_KEY
R2_PUBLIC_BASE_URL
```

## Public pages

```text
/
/articles/{slug}
/categories/{slug}
```

Example sub-path deployment:

```text
https://oddity.example.com/oddity/
```

The nginx sub-path location is tracked in `deploy/nginx-location.conf`.

## Admin UI

Open `/oddity/admin/` in a sub-path deployment. The normal browser flow uses `EDITOR_USER`
and `EDITOR_PASSWORD`; HTTP Basic with `ADMIN_API_KEY` remains an emergency
fallback for operators.

Editor session variables:

```text
EDITOR_USER
EDITOR_PASSWORD            # at least 12 characters, distinct from API keys
SESSION_SECRET             # at least 32 characters
ADMIN_COOKIE_PATH          # /oddity/admin in production
```

## API (current)

```text
GET    /health
GET    /api/v1/articles?slug=&title=  Bearer admin or Hermes
GET    /api/v1/articles/{slug-or-id}  Bearer admin or Hermes
POST   /api/v1/articles          Bearer admin or Hermes
PUT    /api/v1/articles/{id}     Bearer admin or Hermes
DELETE /api/v1/articles/{id}     Bearer admin only
GET    /api/v1/categories
POST   /api/v1/categories        Bearer admin or Hermes
```

Required write credentials:

```text
ADMIN_API_KEY
HERMES_API_KEY
```

## Quick Start (Docker Compose)

Requirements: Docker Engine with the Compose plugin.

1. Copy the environment template and replace every placeholder with real
   secrets (all four are required to start):

   ```bash
   cp .env.example .env   # then edit .env; never commit it
   ```

   Recommended secret generation (each value must be unique):

   ```bash
   openssl rand -hex 32   # use one output per ADMIN_API_KEY / HERMES_API_KEY / SESSION_SECRET
   ```

2. Build and start the service (binds to `127.0.0.1:8890` only):

   ```bash
   docker compose up -d --build
   docker compose logs -f oddity
   curl http://127.0.0.1:8890/health
   ```

All state (SQLite database + uploaded media) is kept in the `mythblog-data`
named volume mounted at `/data` inside the container. Back up that volume to
back up the blog. Put nginx or another reverse proxy in front of the loopback
port for public access — see `deploy/nginx-location.conf` for a sub-path
example.

Useful operations:

```bash
docker compose down            # stop; the data volume is preserved
docker compose pull && docker compose up -d   # upgrade (release image)
docker volume inspect mythblog-data
```

## Releases

Every `v*` tag is built by GitHub Actions (`.github/workflows/release.yml`)
and published in two forms:

- **Release binaries** — static Linux builds attached to the GitHub release:
  `oddity_<version>_linux-amd64.tar.gz`, `oddity_<version>_linux-arm64.tar.gz`
  plus `SHA256SUMS` and per-file `.sha256` checksums. Verify and run:

  ```bash
  VERSION=v0.1.0
  curl -fLO "https://github.com/mopga/mythblog/releases/download/${VERSION}/oddity_${VERSION}_linux-amd64.tar.gz"
  curl -fLO "https://github.com/mopga/mythblog/releases/download/${VERSION}/oddity_${VERSION}_linux-amd64.tar.gz.sha256"
  sha256sum -c "oddity_${VERSION}_linux-amd64.tar.gz.sha256"
  tar xzf "oddity_${VERSION}_linux-amd64.tar.gz"
  # configure environment variables (see .env.example), then:
  ./oddity
  ```

- **Docker images** on GHCR, built for `linux/amd64` and `linux/arm64`:

  ```text
  ghcr.io/mopga/mythblog:v0.1.0
  ```

  To deploy a published image instead of a local build, replace the
  `build:` block in `docker-compose.yml` with
  `image: ghcr.io/mopga/mythblog:v0.1.0` (the tag pinned in the file's
  comment) and run `docker compose up -d`.

## Development

Requires Go 1.27 or newer.

```bash
export PATH=/usr/local/go/bin:$PATH
go test ./...
go build -o bin/oddity ./cmd/oddity
ODDITY_DB_PATH=./data/oddity.db ./bin/oddity
curl http://127.0.0.1:8890/health
```

Default address: `127.0.0.1:8890`.

Secrets are supplied through environment variables (see `.env.example`) and
must never be committed.

## License

MIT — see [LICENSE](LICENSE).
