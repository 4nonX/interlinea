---
title: Try the team server
description: Start a team server with Docker on your own computer in one command, and where to go from there.
---

A team server keeps termbases, memories and projects that several people
share, with sign-in and roles. To try one on your own computer, with
[Docker](https://docs.docker.com/get-docker/) installed:

```sh
docker run -d --name interlinea -p 8080:8080 -v interlinea:/data \
  -e INTERLINEA_ADMIN_USER=admin -e INTERLINEA_ADMIN_PASSWORD='a long password' \
  ghcr.io/4nonx/interlinea-server:latest
```

Open `http://localhost:8080` and sign in as `admin` with that password.
The desktop app signs in to it under **Settings**, with the server's
address.

Without Docker, unpack `interlinea-server-<version>-<system>-<processor>`
from the [release](/#download) and start `interlinea-server -web web`.

## For real use

A server that others reach needs HTTPS, backups and a plan for upgrades.
[Running a team server](/docs/manual/running/) covers reverse proxies,
PostgreSQL, identity providers, backups and upgrades;
[sign-in and roles](/docs/manual/server/) explains accounts and permissions.
