# Team server

How the team server works: who signs in how, who may do what, and how the
API is laid out. Setting one up (HTTPS, a reverse proxy, an identity
provider, backups, upgrades) is in [running.md](running.md); every route is
in the [API reference](api.md).

## What it keeps

`interlinea-server` is first of all where a team shares its terminology:
master termbases everyone translates with, and the review that new and
changed entries pass before they reach them. It keeps projects, memories
and accounts for the team as well. It runs on SQLite files (`-data`, the
default) or PostgreSQL (`-postgres`, each store in its own schema), and
serves the web app from `-web`. Every option is also an environment variable (`-addr` is
`INTERLINEA_ADDR`); `interlinea-server -h` lists them.

```
docker build -t interlinea-server .
docker run -p 8080:8080 -v interlinea:/data \
  -e INTERLINEA_ADMIN_USER=admin -e INTERLINEA_ADMIN_PASSWORD='a long password' interlinea-server
```

`compose.yaml` runs it with PostgreSQL. Without `INTERLINEA_ADMIN_*`, a server
with no accounts logs a setup token, and `POST /api/setup` makes the first
administrator with it. [docs/running.md](running.md) covers HTTPS and
reverse proxies, identity providers, backups (`interlinea-server backup`),
upgrades and sizing; [docs/api.md](api.md) is the API reference, also
served as OpenAPI 3.1 at `/api/openapi.json`.

## Signing in

`POST /api/auth/password` with a username and password
returns a token for the `Authorization: Bearer` header; the stores keep only
its hash. The web app the server serves asks for an HttpOnly cookie instead
(`X-Interlinea-Session: cookie`), so the browser keeps no token; requests
signed in with it carry the `X-Interlinea` header. After ten wrong passwords an account waits 15 minutes, and one
address gets twenty attempts a minute. With `-oidc-issuer` and
`-oidc-client-id` (Entra ID, Google, Keycloak, any OpenID Connect provider;
the redirect address is `<public URL>/api/auth/oidc/callback`), the apps
start a sign-in with `POST /api/auth/oidc/start`, open the address it gives,
and get a one-time code back in the fragment of their return address (the
server, an allowed web app, or an app listening on `127.0.0.1`), which
`POST /api/auth/exchange` turns into a token. Tokens never appear in an
address. People link their provider sign-in to their account themselves, an
administrator links it, or `-oidc-create-users` makes accounts (without
roles) on first sign-in. A link someone begins themselves
(`POST /api/me/identities`) comes back with a one-time code
(`#interlinea-link=`) that their own account confirms
(`POST /api/me/identities/confirm`), so nobody can link their sign-in to
another person's account by sending them the provider's address.

## Who may do what

Roles are read < suggest < edit < approve < manage, on
one project, memory or termbase, on all of a kind (`*`), or on everything.
Administrators may do everything; managers decide who may use what they
manage. Making projects takes manage on all projects; the maker manages the
project and what was made with it. Work assigned in a project lets people
see it, save their segments (reviewers correct the translators'), add terms
to its termbase and look up in its memories, with no grant on the memories
themselves. What someone may not see answers 404.

A project's managers choose the memories and termbases it uses among those
they may use themselves: read on what it looks things up in, edit on a
memory it stores translations in and on the master new terms go into.
Those who make projects (manage on all projects) choose among all of them,
as the catalog offers them. A project is a way into what it uses, so this
keeps a project from opening a memory or termbase to someone without a
grant on it.

A role is a set of permissions. The five that come with the server each
include the ones before them. Administrators define others on the Accounts
page, in the browser and in the desktop app, for combinations the ladder
does not have: a reviewer who reads, suggests and approves but does not
edit, for one. A role of one's own is given like any other, and changing
what it allows changes it at once for everyone who has it; one that people
have is not deleted. Grants add up: what someone may do with a termbase is
everything their roles on it, on all termbases and on everything allow
together. `GET /api/roles`, `PUT /api/roles/{name}` and
`DELETE /api/roles/{name}` are the API for it.

## Writes and errors

Writes name the version they were made from (`If-Match` or `?version=`)
and are refused with 409 `conflict` when someone saved in between. Errors are
`{"code", "message", "problems": [{"field", "message"}]}`.

## The API

| Routes | |
|---|---|
| `/api/me`, `/api/people`, `/api/users`, `/api/grants` | the signed-in person, their sign-ins and identities; whom work can go to; accounts (administrators); roles |
| `/api/catalog`, `/api/templates` | memories and termbases to set projects up with; project templates |
| `/api/projects/{id}` and below | settings, files (bytes in, originals out), segments, pre-translation, lookup and concordance in the project's memories, its terminology, assignments, presence, change log |
| `/api/memories/{name}` and below, `/api/memoryfiles/inspect` | units, lookup, concordance, import (TMX, Trados Studio, Wordfast) and TMX export, change log; what a memory file holds |
| `/api/termbases/{name}` and below | entries, history, revert, merge, duplicates, lookup, the layered view of a project termbase, import and export (TBX, MultiTerm, CSV, TSV, Excel), review rules, merge requests |
| `/api/merge-requests/{id}` and below | items, samples, progress, decisions, reviewers, closing, reverting |
| `/api/events` | server-sent events for every change the person may see; reconnecting with `Last-Event-ID` sends what was missed |

Creating a project (`POST /api/projects`) makes its own memories and its
termbase (layered over the masters, or on its own) with it, and removes
them again if the project is refused. The apps send a translatable file with
its segments; the server reads documents and XLIFF files itself and refuses
segments that are not the file's (and checks that a text file's segments
are its text), keeping the file's own source text and locks. Memories and termbases a project or
template uses are not deleted under it, and renaming a termbase renames it
in their settings.
