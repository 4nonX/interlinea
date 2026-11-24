# Connecting assistants and other programs

Interlinea has two doors for other programs. Both are open on the team
server and on the core the desktop app runs on your computer, and both work
as a person: a program has that person's rights and no more, and what it
saves is recorded under that person's name.

- The **HTTP API** does everything the apps do. It is described in
  [api.md](api.md) and, for tools that read it, in `server/openapi.json`.
- The **MCP endpoint** lets an AI assistant work on projects through the
  Model Context Protocol: read segments, look up the memory and the terms,
  save translations, leave comments.

## An access token

A program signs in with a token. Make one for each program, so each can be
ended on its own:

```sh
curl -X POST https://interlinea.example.org/api/me/tokens \
  -H "Authorization: Bearer $YOUR_SIGN_IN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "My assistant", "days": 90}'
```

The answer holds the token once; it can't be read again. A token lasts 90
days unless `days` says otherwise (1 to 366). Tokens are listed with your
sign-ins (`GET /api/me/tokens`) and ended there
(`DELETE /api/me/tokens/{id}`).

## The MCP endpoint

The endpoint is `/mcp` on the server's address, for example
`https://interlinea.example.org/mcp`. It speaks the HTTP transport of the
protocol: every request is a POST with one JSON-RPC message and gets one
JSON message back. It keeps no stream open and sends nothing on its own.
Send the token in the `Authorization` header as `Bearer <token>`.

In a client that takes a JSON description of its servers, that is:

```json
{
  "mcpServers": {
    "interlinea": {
      "type": "http",
      "url": "https://interlinea.example.org/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

### Tools

| Tool | What it does |
| --- | --- |
| `list_projects` | The projects the person may see, with their languages and files. |
| `list_segments` | Segments of a file in a target language: source, translation, status, notes, comments. By status and in pages. |
| `save_translation` | Saves a segment's translation as a draft, or confirmed with `confirm`. |
| `search_memory` | Earlier translations of a sentence like this one, with their likeness in percent. |
| `search_concordance` | Units of the memories that hold given words, in the source or the translations. |
| `find_terms` | The terms of the project's termbases in a text, with translations and status. |
| `add_comment` | Leaves a comment on a segment for the people working on the project. |

### Tags

A segment's formatting and placeholders are tags. In the text the tools
give and take, a pair is written `<1>…</1>` and a tag on its own `<2/>`; a
`<` that is text is written `&lt;` and an `&` as `&amp;`. A translation
keeps the source's tags. One with a tag the source does not have is
refused, and the answer says which tags the source has.

### What a tool can't do

A tool's failure comes back as its result with `isError` set and the
reason in words, so an assistant can read it and act on it: a segment that
is locked or assigned to someone else, a translation saved by someone else
in the meantime, a project the person has no access to.

## What is sent where

Nothing leaves Interlinea through these doors unless a program with a
token asks for it. An assistant you connect reads the segments, memory
matches and terms it asks for; where that text goes then is up to the
assistant and the service behind it. Give a program a token of an account
that sees only the projects it should work on.
