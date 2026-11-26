# Connecting assistants, translation services and other programs

Interlinea asks a translation service of your choice for suggestions
(DeepL, Google, Microsoft, OpenAI, Claude, or one of your own), and
has two doors for other programs to come in. Both are open on the team
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
ended on its own, in Settings under Access for programs, or over the API:

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

## Translation services

The editor asks a translation service for a suggestion when you ask for
one. A service is added in Settings under Translation services: choose it,
paste the key of your own account, and give it a name. The services built
in:

| Service | What it needs |
|---|---|
| DeepL | a key of the free or the paid API ([deepl.com/pro-api](https://www.deepl.com/pro-api)); a free key ends in `:fx` |
| Google Cloud Translation | an API key of a Google Cloud project with the Cloud Translation API on |
| Microsoft Translator | a key of an Azure Translator resource, and its region unless the key is global |
| OpenAI or a compatible service | a key, and a model chosen from those the key may use; with an address, any service that speaks the same API (Mistral, a model run on your own computer) |
| Anthropic (Claude) | a key, and a model chosen from those the key may use |

"List the models" asks the service which models the key may use, so the
list is always the service's own. Tags are kept: DeepL, Google and
Microsoft get them as markup they keep, and a language model is asked to
keep them as they are. A translation that loses a tag is taken without
tags and marked, so the translator puts them in by hand.

On a team server an administrator adds the services; the server sends the
requests, so the key stays there and no app ever gets it. For work on your
own computer the desktop app's core sends them. In the web app without a
server the browser sends them itself, with the key kept in the browser; a
service must allow requests from a web page for that, which DeepL does not,
so DeepL is added in the desktop app or on a team server.

Over the API, `GET /api/providers/services` lists the services built in
and what each needs, and `POST /api/providers/models` lists the models a
key may use (administrators).

## A translation service of your own

The editor can also ask any machine translation engine or language model
that answers at an address the way described here. A service that speaks
another protocol needs a small program in front of it that takes this
request and gives this answer.

Such a service is added as "A service of your own": a name, the address,
and optionally a header that signs each request (for example
`Authorization` with the value `Bearer <key>`). It is sent from where the
built-in services are sent from, as above; from the web app without a
server, the service must allow requests from the page (CORS).

The request is a POST with this JSON body:

```json
{
  "sourceLang": "de-DE",
  "targetLang": "en-GB",
  "segments": [{ "id": "1", "text": "Der <1>Mieter</1> zahlt." }]
}
```

The answer has status 200 and this JSON body:

```json
{
  "translations": [{ "id": "1", "text": "The <1>tenant</1> pays." }]
}
```

Tags are written as described under Tags above. A translation should keep
them; one that does not have each of the source's tags as often as the
source is taken without tags and marked, so the translator puts them in by
hand. Any other status is an error, and the first 200 characters of its
body are shown to the person who asked.

Over the API, `GET /api/providers` lists the services,
`PUT /api/providers/{name}` and `DELETE /api/providers/{name}` are for
administrators, and `POST /api/providers/{name}/translate` asks one for up
to 50 segments.

## Grammar services

A grammar checker of your choice reads the translations of the projects
that name it. Interlinea asks it the way the LanguageTool HTTP API is
asked, so a LanguageTool server you run yourself works as it is, and so
does anything else that answers the same request.

A service is added in Settings under Grammar services, like a translation
service: a name, the address of its check request (for a LanguageTool
server, `http://localhost:8081/v2/check`), and optionally a header that
signs each request. A project names its service under Quality checks.
On a team server an administrator adds the services and the server sends
the requests. For work on your own computer the desktop app's core sends
them; in the web app without a server the browser does, so the service
must allow requests from the page (a LanguageTool server is started with
`--allow-origin`).

The request is a POST with a form body
(`application/x-www-form-urlencoded`):

```
text=The translation.&language=en-GB&disabledCategories=TYPOS
```

The answer has status 200 and this JSON body:

```json
{
  "matches": [
    {
      "message": "Use “an” instead of “a” before a vowel sound.",
      "offset": 6,
      "length": 1,
      "replacements": [{ "value": "an" }],
      "rule": { "id": "EN_A_VS_AN", "category": { "id": "MISC", "name": "Miscellaneous" } }
    }
  ]
}
```

`offset` and `length` count UTF-16 code units of the text. The text is
the translation without its tags. Spelling is not asked for, and a finding
of the category `TYPOS` is left out: the project's dictionary and its own
words decide what is spelled right. A service that refuses a language with
a region (`pt-AO`) is asked again for the language alone (`pt`).

In the editor the service reads the open segment's translation a moment
after the last change to it, and again when the segment is confirmed, so
its findings are kept with the confirmed segment as warnings. A finding
is listed with the checks, with up to three replacements that go into the
translation at a click; where the place has a tag in it, the change is
left to the translator.

Over the API, `PUT /api/providers/{name}` with `"kind": "check"` adds a
grammar service, `POST /api/providers/{name}/check` asks one about a text
of up to 20,000 bytes, and the quality report of a file
(`GET /api/projects/{id}/qa/report`) has the service's findings in a
project that names one.

## What is sent where

In a project that names a grammar service, the translation of the open
segment goes to that service while it is worked on, and a file's
translations go to it when the file's quality report is asked for. In a
project that names none, nothing goes to any grammar service.

A segment goes to a translation service when someone asks that service
for a suggestion for it, and not before. Nothing leaves Interlinea through
the two doors unless a program with a token asks for it. An assistant you connect reads the segments, memory
matches and terms it asks for; where that text goes then is up to the
assistant and the service behind it. Give a program a token of an account
that sees only the projects it should work on.
