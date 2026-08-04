# deploy/office

Docker Compose stack for `ovl-office`, per architecture handoff
section 12.1.

`docker-compose.yml` defines two services: `postgres` (always) and
`office` (behind the `full` profile — see below). An OIDC provider
(Keycloak or Authentik — either is spec-compliant per the architecture
doc) is deferred until the office-roles/auth checklist item; see
PROJECT.md's Phase 3 decisions log for why. The API-key-gated GraphQL/CSV
data API (Phase 6, `PROJECT.md`) is a separate, already-built auth
surface, not OIDC.

## Local development

Only Postgres runs in a container; `ovl-office` itself runs on the host
via `make run-office` (or `go run ./office`) against it, so you get fast
iteration and native debugging:

```sh
docker compose -f deploy/office/docker-compose.yml up -d
export OVL_OFFICE_DB_DSN='postgres://ovl:ovl@localhost:5432/ovl_office?sslmode=disable'
make run-office
```

For integration tests in `office/store` and `office/httpapi`, point
`OVL_TEST_DATABASE_URL` at the same instance (a distinct database/schema
is not currently required — tests only run migrations and clean up their
own fixtures).

## Real deployment (containerized `ovl-office`)

`office/Dockerfile` builds `ovl-office` as a single image (Vite build of
`web/office` embedded into the Go binary, same shape as the native
binary). The `office` service in `docker-compose.yml` builds and runs it,
but sits behind Compose's `full` profile so it never starts as a side
effect of the local-dev command above — a bare `docker compose up -d`
still only starts Postgres:

```sh
docker compose -f deploy/office/docker-compose.yml --profile full up -d --build
```

This brings up Postgres and `ovl-office` together: `ovl-office` listens
on `:8080`, talks to the `postgres` service over the Compose network (no
DSN to set by hand), and persists its own filesystem state (attachments)
in the `office-data` named volume. Put a reverse proxy (Caddy, nginx, a
cloud load balancer) in front of `:8080` for TLS — see the root
[`README.md`](../../README.md#running-ovl-office-in-production-docker-compose)
for a minimal `Caddyfile` example and the API-key/GraphQL walkthrough for
whoever operates this stack once it's up.

Once that TLS front-end is in place, uncomment
`OVL_OFFICE_SECURE_COOKIES: "true"` in `docker-compose.yml` (or pass
`-secure-cookies`) so the session cookie is marked `Secure` and never
leaves the browser over plain HTTP. Keep it off until then — a `Secure`
cookie sent over a plain-HTTP hop is silently dropped, which looks like
login "not sticking." The cookie is already `HttpOnly` and
`SameSite=Strict` regardless.

Operational note: office sessions are held in memory, so restarting the
`ovl-office` container logs every signed-in user back out. That is
expected for this single-node stack; users just sign in again.
