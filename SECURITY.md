# Security

## Reporting a problem

Please report a security problem privately, not in an open issue:
[report a vulnerability](https://github.com/4nonX/interlinea/security/advisories/new)
on GitHub (the Security tab, "Report a vulnerability"). Only the
maintainers see the report, and the fix can be prepared and released
before the problem is public.

Say what is affected (the team server, the desktop app, the browser
client, the command line), the version, and how to make the problem
happen. A small file or a few requests that show it help most.

Once a fixed version is released, the advisory is published with it.

## Versions

Fixes go into the newest release. Interlinea is at version 0.x: there are
no older lines that get fixes of their own, so update to the newest
release.

## What counts

Anything that lets someone read, change or delete what they may not, on a
team server or through a file opened in the apps: getting around sign-in
or roles, reaching another person's projects, memories or termbases,
running code or reading files on a server or computer through a crafted
document, package or request, and leaking sign-in tokens or keys of
translation services.

A team server that its administrator set up without HTTPS, or exposed
without the protections [running a team server](docs/running.md)
describes, is not a problem of Interlinea's code.
