# Running the team server

`interlinea-server` is one program. It keeps accounts, projects, translation
memories and termbases, serves the web app, and answers the desktop and web
apps over HTTP. This guide is for whoever runs it: setting it up, keeping it
reachable and safe, backing it up and upgrading it. The API itself is in
[api.md](api.md).

## Where the data lives

The server keeps four stores: accounts (with sign-ins, roles and provider
identities), projects (settings, the uploaded files with their bytes,
segments, assignments, presence), memories and termbases. Each has a change
log, which the live feed and the apps' syncing read.

- **SQLite** (the default): four files in the data folder (`-data`,
  `INTERLINEA_DATA`; `/data` in the container image): `accounts.db`,
  `projects.db`, `memories.db`, `termbases.db`. They are in WAL mode, so
  next to each file there may be a `-wal` and a `-shm` file while the server
  runs; they belong to it.
- **PostgreSQL** (`-postgres`, `INTERLINEA_POSTGRES`, a connection string):
  one database, with each store in its own schema (`interlinea_accounts`,
  `interlinea_projects`, `interlinea_memories`, `interlinea_termbases`). The
  server makes the schemas, and the `pg_trgm` extension in `public`, at its
  first start; the database user needs the right to do that once (or an
  administrator makes the extension beforehand).

SQLite needs nothing else and suits a team on one machine. PostgreSQL suits
a team that already runs it, wants its backups and replication, or works on
the server from many places at once: SQLite lets one write happen at a time
per store (others wait up to ten seconds), PostgreSQL many.

## Starting it

With Docker, from the released image (`:latest`, or a version such as
`:0.1.0`):

```
docker run -d --name interlinea -p 8080:8080 -v interlinea:/data \
  -e INTERLINEA_ADMIN_USER=admin -e INTERLINEA_ADMIN_PASSWORD='a long password' \
  ghcr.io/4nonx/interlinea-server:latest
```

While the repository is private, so is the image: `docker login ghcr.io`
first, with a GitHub token that may read packages. `docker build -t
interlinea-server .` builds the same image from the source.

`compose.yaml` runs it with PostgreSQL:

```
INTERLINEA_ADMIN_PASSWORD='a long password' POSTGRES_PASSWORD='another one' docker compose up -d
```

Without Docker, unpack `interlinea-server-<version>-<system>-<processor>`
from the release and start `interlinea-server -web web` in it. From the
source: build it (`go build ./cmd/interlinea-server`), build the web app
(`make wasm`, then `npm ci && npm run build` in `prototype/`) and start it
with `-web prototype/dist`.

**The first administrator.** With `INTERLINEA_ADMIN_USER` and
`INTERLINEA_ADMIN_PASSWORD` set, the server makes that account at its first
start, while it has no accounts; afterwards the variables do nothing and can
be removed. Without them, the server logs a setup token at its first start,
and the web app's setup page (or `POST /api/setup`) makes the first
administrator with it. Passwords are 12 characters or more.

### Settings

Every option is also an environment variable; `interlinea-server -h` lists
them.

| Option | Variable | |
|---|---|---|
| `-addr` | `INTERLINEA_ADDR` | address to listen on (`:8080`) |
| `-data` | `INTERLINEA_DATA` | folder of the SQLite files (`data`) |
| `-postgres` | `INTERLINEA_POSTGRES` | PostgreSQL connection string, instead of SQLite |
| `-public-url` | `INTERLINEA_PUBLIC_URL` | the address people reach the server at, `https://...` |
| `-web` | `INTERLINEA_WEB` | folder of the built web app, served at `/` |
| `-origins` | `INTERLINEA_ORIGINS` | other web app addresses allowed to call the API, comma-separated |
| `-tls-cert`, `-tls-key` | `INTERLINEA_TLS_CERT`, `INTERLINEA_TLS_KEY` | serve HTTPS itself |
| `-trust-proxy` | `INTERLINEA_TRUST_PROXY` | behind a reverse proxy: take the client's address from `X-Forwarded-For` and HTTPS from `X-Forwarded-Proto` |
| `-token-days` | `INTERLINEA_TOKEN_DAYS` | days a sign-in lasts (30) |
| `-log-json` | `INTERLINEA_LOG_JSON` | log JSON lines (on in the image) |
| `-oidc-issuer`, `-oidc-client-id` | `INTERLINEA_OIDC_ISSUER`, `INTERLINEA_OIDC_CLIENT_ID` | sign-in through an identity provider |
| | `INTERLINEA_OIDC_CLIENT_SECRET` | the provider's client secret (only in the environment) |
| `-oidc-name` | `INTERLINEA_OIDC_NAME` | the provider's name on the sign-in button |
| `-oidc-scopes` | `INTERLINEA_OIDC_SCOPES` | scopes asked for (`openid,email,profile`) |
| `-oidc-create-users` | `INTERLINEA_OIDC_CREATE_USERS` | make an account, without roles, for people the provider signs in |

## HTTPS and a reverse proxy

Run the server behind HTTPS: passwords and sign-ins travel with every
request. Either give it a certificate (`-tls-cert`, `-tls-key`) or put it
behind a reverse proxy that ends HTTPS, and then:

- set `-public-url` to the address people use (`https://interlinea.example`);
- set `-trust-proxy`, so sign-in limits count per person's address and the
  web app's cookie is marked Secure. Only set it when every request comes
  through the proxy: the server then believes `X-Forwarded-For`;
- let the live feed through: `/api/events` is a long-lived response
  (server-sent events, with a keep-alive every 25 seconds). Turn off
  response buffering for it and allow idle reads of a minute or more;
- allow large requests: a file is uploaded as base64 in JSON, up to 256 MB
  per request (other JSON requests up to 4 MB).

For nginx:

```
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
    client_max_body_size 260m;
}
location /api/events {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_buffering off;
    proxy_read_timeout 1h;
}
```

The web app the server serves signs in with an HttpOnly, SameSite=Strict
cookie scoped to `/api/`, which its scripts can't read. A web app served
from another address (listed in `-origins`) and the desktop app use tokens.

## Signing in through an identity provider

Register Interlinea at the provider (Entra ID, Google, Keycloak, any OpenID
Connect provider) as a web application with the redirect address
`<public URL>/api/auth/oidc/callback`, and start the server with
`-oidc-issuer`, `-oidc-client-id` and, for a confidential client,
`INTERLINEA_OIDC_CLIENT_SECRET`. `-public-url` is needed for this.

People then link their provider sign-in to their account in the web app, an
administrator links it (Accounts), or `-oidc-create-users` makes an account
at a person's first sign-in. Such accounts have no roles until someone gives
them some. Sign-ins in progress are kept in the accounts store, so a restart
in the middle of one doesn't break it.

## Backups

Back up the stores together, so accounts, projects, memories and termbases
match.

**SQLite.** `interlinea-server backup` copies the four files into a new
folder while the server keeps running. Each copy is consistent (SQLite's
`VACUUM INTO`) and compact, without its write-ahead log:

```
interlinea-server backup -data /srv/interlinea/data /srv/backups/interlinea-2026-11-23
docker exec interlinea interlinea-server backup /data/backups/2026-11-23
```

(In the container, the second form writes into the data volume; copy the
folder out from there.) Copying the `.db` files with `cp` is only safe while
the server is stopped, and then with their `-wal` files.

To restore: stop the server, move the data folder's files away (the `.db`,
`-wal` and `-shm` files), copy the backup's four files in, and start it.

**PostgreSQL.** Back up the four schemas with `pg_dump`:

```
pg_dump --format=custom --schema='interlinea_*' --file=interlinea.dump "$INTERLINEA_POSTGRES"
```

To restore into an empty database: make the `pg_trgm` extension
(`CREATE EXTENSION pg_trgm SCHEMA public`), then
`pg_restore --dbname=... interlinea.dump`, and start the server. Your
database's own backups (base backups with WAL archiving) work as well.

Sign-in tokens are in the accounts store as hashes only; a backup holds no
usable tokens, but it holds everything else, including the uploaded files.
Keep backups as private as the server.

## Upgrades

1. Back up (above).
2. Stop the server, replace the program (or the image), start it.

At start, each store brings its schema up to date by itself, in a
transaction: a failed upgrade leaves the store as it was, and the server
says why and doesn't start. A store whose schema is newer than the program
refuses to open: going back to an older version means restoring the backup
made before the upgrade. Sign-ins survive restarts and upgrades.

The web app is served by the server, so it updates with it; people with the
page open get the new one on their next reload.

**What is tested.** Every release leaves a data folder with one of
everything in it (accounts, roles, a project with translations, memories,
termbases with a history, a deleted entry and a merge request) in
`server/testdata/releases`. A test opens each of them with the current
code, checks through the API that everything is still there, and then
works on it. The steps of each store's schema are recorded, and a test
fails when a step that databases have already taken is changed: a change
to the schema is always a new step.

## Sizing

The server itself holds little in memory: the sign-in limits, the live
feed's connections and a list of projects it keeps for two seconds; a
request's work is done when it answers. What grows is the stores:

- a project keeps its uploaded files with their bytes, and its segments for
  each target language;
- a memory takes, in a test with 50,000 sentence-length units, about 2.3 KB
  per unit on disk (around ten times its text), most of it the indexes for
  fuzzy matching and concordance;
- termbases are small next to memories.

When an editor opens a server project, the server sends only the units of
each memory that match the project's segments (50 % or more), and
concordance searches run on the server, so large memories don't travel to
the apps.

The live feed polls the stores' change logs once a second, whatever the
number of people connected, and sends each person only what they may see.

Run one server process per set of stores: the limits on sign-in attempts
are kept in memory, and running several processes against one PostgreSQL
database has not been tested.

## Watching it

- `GET /healthz` answers `ok` while the server runs;
  `interlinea-server healthcheck` asks it (the image's health check).
- The log says when the server starts and stops, which stores it opened,
  refused and throttled sign-ins (with the username and address), removed
  expired sign-ins (hourly), and errors. With `-log-json`, each line is a
  JSON object.
- When the server has no accounts, the log carries the setup token: treat
  that log line as a secret until the first administrator is made.
