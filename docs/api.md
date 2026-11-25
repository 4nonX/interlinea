# Team server API

<!-- Written by `go run ./cmd/interlinea-apidoc` from the server's source. Do not edit. -->

The API of the Interlinea team server: accounts and roles, projects and their files, segments and work, translation memories, termbases and the review of their changes, and a live feed of changes.

Requests and answers are JSON (UTF-8), except file uploads and downloads, which say their type. Every answer that is not a success has an Error body with a stable code. Writes to things that have a version (projects, segments, entries, units, accounts, templates) carry the version they were made from, in If-Match or ?version=; a write made from an older version is refused with 409 conflict, and the app reloads.

Sign in with a password (/api/auth/password), through the identity provider (/api/auth/oidc/start), or as the first administrator (/api/setup). The answer's token goes in "Authorization: Bearer <token>". The web app the server serves itself asks for a cookie instead (X-Interlinea-Session: cookie on the sign-in): it is HttpOnly and SameSite=Strict, and every request it signs in must carry the X-Interlinea header.

Roles on projects, memories and termbases are read < suggest < edit < approve < manage, given per resource or for all of a kind; administrators may do everything.

The desktop app's core, interlinea-core, serves the same API on the person's own computer, for them alone: on the loopback interface, with one account and a token it hands the app when it starts.

The same description is served as OpenAPI 3.1 at `/api/openapi.json` (and kept in `server/openapi.json`).

Every route may also answer with an [Error](#error): 422 for a request with problems, 429 when there are too many sign-in attempts, 500 when the server failed.

## Routes

- [Signing in](#signing-in)
- [The signed-in person](#the-signed-in-person)
- [Accounts and roles](#accounts-and-roles)
- [Projects](#projects)
- [Files, checks and spelling](#files-checks-and-spelling)
- [Memories](#memories)
- [Termbases](#termbases)
- [Merge requests](#merge-requests)
- [Live feed](#live-feed)
- [Other](#other)
- [Types](#types)

## Signing in

### `POST /api/auth/exchange`

Trades the one-time code from a provider sign-in for a sign-in: a token, or the web app's cookie when asked (X-Interlinea-Session: cookie).

A code works once and for a few minutes.

Access: anyone with a code.

- Parameters: `X-Interlinea-Session` (header)
- Body: {`code`: string}
- Answer: 200 OK: [SigninResult](#signinresult)
- Errors: 422 (see [Error](#error))

### `GET /api/auth/oidc/callback`

Takes the browser back from the identity provider and sends it on to the app's return address, with a one-time code (#interlinea-code=), a linked identity (#interlinea-linked=) or an error (#interlinea-error=) in the fragment.

A sign-in that expired or was used already gets a page that says so.

Access: anyone; the state ties it to a sign-in begun at oidc/start.

- Parameters: `state` (query), `error` (query), `error_description` (query), `code` (query)
- Answer: 303 See Other
- Errors: 422 (see [Error](#error))

### `POST /api/auth/oidc/start`

Begins a sign-in through the identity provider and answers with the provider's address to open in a browser.

The provider sends the browser to the callback, which sends it on to the return address with a one-time code in the fragment (#interlinea-code=...); the app trades the code for a sign-in at /api/auth/exchange.

Access: anyone, when the server has an identity provider.

- Body: [OidcStartRequest](#oidcstartrequest)
- Answer: 200 OK: map of string
- Errors: 422 (see [Error](#error))

### `POST /api/auth/password`

Signs in with a username and password.

The answer has a token for the Authorization header, or, when the request has X-Interlinea-Session: cookie, sets the web app's HttpOnly cookie instead and leaves the token out. Attempts are limited per address and per account.

Access: anyone.

- Parameters: `X-Interlinea-Session` (header)
- Body: [PasswordSignin](#passwordsignin)
- Answer: 200 OK: [SigninResult](#signinresult)
- Errors: 422 (see [Error](#error))

### `POST /api/auth/signout`

Ends the sign-in the request was made with, and clears the web app's cookie if it signed in with it.

Access: signed in.

- Answer: 204 No Content
- Errors: 401, 403, 422 (see [Error](#error))

### `GET /api/openapi.json`

Returns this description of the API, in OpenAPI 3.1.

Access: anyone.

- Answer: 200 OK: `application/json`

### `GET /api/server`

Says what the server is: its version, the identity provider people can sign in with, and whether it is waiting for its first administrator.

Access: anyone.

- Answer: 200 OK: [ServerInfo](#serverinfo)

### `POST /api/setup`

Makes the first administrator, with the setup token the server logged at its first start.

It works only while the server has no accounts. Like the other sign-ins it answers with a token, or sets the web app's cookie when asked to (X-Interlinea-Session: cookie).

Access: anyone with the setup token; attempts are limited per address.

- Parameters: `X-Interlinea-Session` (header)
- Body: [SetupRequest](#setuprequest)
- Answer: 200 OK: [SigninResult](#signinresult)
- Errors: 422 (see [Error](#error))

## The signed-in person

### `GET /api/me`

Returns the signed-in person's account, roles and linked provider sign-ins.

Access: signed in.

- Answer: 200 OK: [MeResult](#meresult)
- Errors: 401, 403 (see [Error](#error))

### `GET /api/me/assignments`

Lists the work given to the signed-in person, in all projects.

Access: signed in.

- Answer: 200 OK: list of [projectdb.Assignment](#projectdbassignment)
- Errors: 401, 403 (see [Error](#error))

### `POST /api/me/identities`

Begins linking a provider sign-in to the signed-in person's account, and answers with the provider's address to open.

The callback sends the browser back with #interlinea-linked= in the fragment.

Access: signed in, when the server has an identity provider.

- Body: [OidcStartRequest](#oidcstartrequest)
- Answer: 200 OK: map of string
- Errors: 401, 403, 422 (see [Error](#error))

### `DELETE /api/me/identities`

Removes a provider sign-in (?issuer=&subject=) from the signed-in person's account.

An account without a password keeps its last one.

Access: signed in.

- Parameters: `issuer` (query), `subject` (query)
- Answer: 204 No Content
- Errors: 401, 403, 422 (see [Error](#error))

### `PUT /api/me/password`

Sets the signed-in person's password, after checking the current one if they have one.

Every other sign-in of theirs ends; this one continues with a new token (or cookie), which the answer carries.

Access: signed in.

- Parameters: `X-Interlinea-Session` (header)
- Body: {`current`: string, `new`: string}
- Answer: 200 OK: [SigninResult](#signinresult)
- Errors: 401, 403, 422 (see [Error](#error))

### `GET /api/me/tokens`

Lists the signed-in person's sign-ins (devices), marking the one the request was made with.

Access: signed in.

- Answer: 200 OK: list of {`createdAt`: string (date-time), `current`: boolean, `expiresAt`: string (date-time), `id`: integer, `lastUsed`: string (date-time), `name`: string, `userId`: integer}
- Errors: 401, 403 (see [Error](#error))

### `POST /api/me/tokens`

Makes an access token for a program that works as the signed-in person: an assistant on the MCP endpoint, a script on the API.

The token is in the answer once and can't be read again; it is listed with the person's sign-ins and ended there.

Access: signed in.

- Body: {`days`: integer, `name`: string}
- Answer: 201 Created: map of any
- Errors: 401, 403, 422 (see [Error](#error))

### `DELETE /api/me/tokens/{id}`

Ends one of the signed-in person's sign-ins.

Access: signed in; only their own sign-ins.

- Parameters: `id` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

## Accounts and roles

### `GET /api/grants`

Lists who has which role on a project, memory or termbase (?kind=&resource=).

Access: administrators, and those who manage the resource.

- Parameters: `kind` (query), `resource` (query)
- Answer: 200 OK: list of [GrantView](#grantview)
- Errors: 401, 403, 422 (see [Error](#error))

### `PUT /api/grants`

Gives someone a role on a project, memory, termbase or everything of a kind (resource *).

Access: administrators, and those who manage the resource (for one resource).

- Body: [authdb.Grant](#authdbgrant)
- Answer: 204 No Content
- Errors: 401, 403, 422 (see [Error](#error))

### `DELETE /api/grants`

Takes a role back (?userId=&kind=&resource=).

Access: administrators, and those who manage the resource.

- Parameters: `kind` (query), `resource` (query), `userId` (query)
- Answer: 204 No Content
- Errors: 401, 403, 422 (see [Error](#error))

### `GET /api/people`

Lists the accounts that are not disabled, by name, to give work to and to name in grants.

Access: signed in.

- Answer: 200 OK: list of [Person](#person)
- Errors: 401, 403 (see [Error](#error))

### `GET /api/users`

Lists all accounts.

Access: administrators.

- Answer: 200 OK: list of [authdb.User](#authdbuser)
- Errors: 401, 403 (see [Error](#error))

### `POST /api/users`

Makes an account.

Access: administrators.

- Body: [NewUserRequest](#newuserrequest)
- Answer: 201 Created: [authdb.User](#authdbuser)
- Errors: 401, 403, 422 (see [Error](#error))

### `GET /api/users/{id}`

Returns an account.

Access: administrators.

- Parameters: `id` (path)
- Answer: 200 OK: [authdb.User](#authdbuser)
- Errors: 401, 403, 404 (see [Error](#error))

### `PATCH /api/users/{id}`

Changes an account's name, email, administrator flag or disabled state; fields left out stay.

Disabling an account ends all its sign-ins. Send the version read (If-Match or ?version=).

Access: administrators.

- Parameters: `id` (path), `If-Match` or `?version=`
- Body: [UserPatch](#userpatch)
- Answer: 200 OK: [authdb.User](#authdbuser)
- Errors: 401, 403, 404, 409, 428, 422 (see [Error](#error))

### `DELETE /api/users/{id}`

Deletes an account with its sign-ins, grants and linked identities.

Access: administrators.

- Parameters: `id` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/users/{id}/grants`

Lists an account's roles.

Access: administrators.

- Parameters: `id` (path)
- Answer: 200 OK: list of [authdb.Grant](#authdbgrant)
- Errors: 401, 403, 404 (see [Error](#error))

### `GET /api/users/{id}/identities`

Lists the provider sign-ins linked to an account.

Access: administrators.

- Parameters: `id` (path)
- Answer: 200 OK: list of [authdb.Identity](#authdbidentity)
- Errors: 401, 403, 404 (see [Error](#error))

### `POST /api/users/{id}/identities`

Links a provider sign-in (issuer and subject; the issuer defaults to the server's provider) to an account.

Access: administrators.

- Parameters: `id` (path)
- Body: {`issuer`: string, `subject`: string}
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `DELETE /api/users/{id}/identities`

Removes a provider sign-in (?issuer=&subject=) from an account.

Access: administrators.

- Parameters: `id` (path), `issuer` (query), `subject` (query)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `PUT /api/users/{id}/password`

Sets or removes an account's password and ends its sign-ins.

Access: administrators.

- Parameters: `id` (path)
- Body: {`password`: string}
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/users/{id}/tokens`

Lists an account's sign-ins.

Access: administrators.

- Parameters: `id` (path)
- Answer: 200 OK: list of [authdb.Token](#authdbtoken)
- Errors: 401, 403, 404 (see [Error](#error))

### `DELETE /api/users/{id}/tokens`

Ends all of an account's sign-ins.

Access: administrators.

- Parameters: `id` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

## Projects

### `GET /api/catalog`

Lists the memories and termbases someone may set up projects with: all of them for those who make projects, otherwise the ones they may read.

Role is what they may do with each directly.

Access: signed in.

- Answer: 200 OK: {`memories`: list of [CatalogMemory](#catalogmemory), `termbases`: list of [CatalogTermbase](#catalogtermbase)}
- Errors: 401, 403 (see [Error](#error))

### `GET /api/projects`

Lists the projects the person may see, with their access to each.

Access: signed in; each project needs read, or work in it.

- Answer: 200 OK: list of {`access`: [Access](#access), `changedAt`: string (date-time), `changedBy`: string, `confirmed`: integer, `createdAt`: string (date-time), `createdBy`: string, `id`: integer, `segments`: integer, `settings`: [project.Settings](#projectsettings), `version`: integer}
- Errors: 401, 403 (see [Error](#error))

### `POST /api/projects`

Makes a project with its own memories and termbase.

If a step fails, what was made before it is removed again.

Access: those who make projects (manage on all projects) and administrators.

- Body: [CreateProjectRequest](#createprojectrequest)
- Answer: 201 Created: [ProjectView](#projectview)
- Errors: 401, 403, 422 (see [Error](#error))

### `GET /api/projects/defaults`

Returns the settings a new project starts with, its terminology kept on this server.

Access: signed in.

- Answer: 200 OK: [project.Settings](#projectsettings)
- Errors: 401, 403 (see [Error](#error))

### `POST /api/projects/preview`

Says what a project would be before it is made, and makes nothing.

It takes what POST /api/projects takes, reads the files the same way, and answers for each target language with the analysis of the files against the memories the setup looks up in, and with what pre-translation would fill and confirm. A setup that could not be made is refused with its problems by field, as it is when the project is made, and so is a name a project has already.

Access: those who make projects (manage on all projects) and administrators.

- Body: [CreateProjectRequest](#createprojectrequest)
- Answer: 200 OK: list of [ProjectPreview](#projectpreview)
- Errors: 401, 403, 422 (see [Error](#error))

### `GET /api/projects/{id}`

Returns a project: its settings, files and status, and the person's access.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Answer: 200 OK: [ProjectView](#projectview)
- Errors: 401, 403, 404 (see [Error](#error))

### `DELETE /api/projects/{id}`

Removes a project.

Its memories and termbase stay: they may be used by other projects, and are deleted on their own.

Access: manage on the project.

- Parameters: `id` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/projects/{id}/analysis`

Analyses a project's files against the memories it looks up in: words by match band, repetitions, and with weights the weighted words, for each target language, or for one language and one file.

Each report lists how every segment was counted, with the score and the memory of its best match.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Body: [AnalysisRequest](#analysisrequest)
- Answer: 200 OK: list of [analysis.Report](#analysisreport)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/projects/{id}/assignments`

Lists the work given out in a project.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Answer: 200 OK: list of [projectdb.Assignment](#projectdbassignment)
- Errors: 401, 403, 404 (see [Error](#error))

### `POST /api/projects/{id}/assignments`

Gives someone a range of segments of a file and language to translate or review, with an optional due date.

Access: manage on the project.

- Parameters: `id` (path)
- Body: [projectdb.Assignment](#projectdbassignment)
- Answer: 201 Created: [projectdb.Assignment](#projectdbassignment)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `PUT /api/projects/{id}/assignments/{aid}`

Changes an assignment.

Managers change anything; the person it is for only reports it done or not done.

Access: manage on the project; the assignee for done.

- Parameters: `id` (path), `aid` (path), `If-Match` or `?version=`
- Body: [projectdb.Assignment](#projectdbassignment)
- Answer: 200 OK: [projectdb.Assignment](#projectdbassignment)
- Errors: 401, 403, 404, 409, 428, 422 (see [Error](#error))

### `DELETE /api/projects/{id}/assignments/{aid}`

Takes work back.

Access: manage on the project.

- Parameters: `id` (path), `aid` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/projects/{id}/changes`

Lists the project's changes after ?since= (a change number), oldest first, at most ?limit=.

Access: read on the project, or work in it.

- Parameters: `id` (path), `since` (query), `limit` (query)
- Answer: 200 OK: {`changes`: list of [projectdb.Change](#projectdbchange), `more`: boolean}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/projects/{id}/comments`

Puts a comment on a segment (?file=, ?lang=, ?segment=): a remark for the others working on the project, which does not go into the document.

The segment's translation and version stay as they are. The answer is the segment with its comments.

Access: read on the project, or work in it.

- Parameters: `id` (path), `file` (query), `lang` (query), `segment` (query)
- Body: {`text`: string}
- Answer: 201 Created: [projectdb.StoredSegment](#projectdbstoredsegment)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `DELETE /api/projects/{id}/comments`

Takes a comment (?comment=) off a segment (?file=, ?lang=, ?segment=).

The answer is the segment with the comments that are left.

Access: the comment's author; manage on the project for anyone's.

- Parameters: `id` (path), `file` (query), `lang` (query), `segment` (query), `comment` (query)
- Answer: 200 OK: [projectdb.StoredSegment](#projectdbstoredsegment)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/projects/{id}/concordance`

Searches the memories the project searches with for the language.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Body: [ConcordanceRequest](#concordancerequest)
- Answer: 200 OK: list of [ConcordanceHit](#concordancehit)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/projects/{id}/files`

Adds a file to a project.

A translatable file comes with its segments for each target language; the server reads the file itself and refuses segments that are not the file's. Send the version read (If-Match or ?version=).

Access: manage on the project.

- Parameters: `id` (path), `If-Match` or `?version=`
- Body: [FileUpload](#fileupload)
- Answer: 200 OK: [ProjectView](#projectview)
- Errors: 401, 403, 404, 409, 428, 422 (see [Error](#error))

### `DELETE /api/projects/{id}/files`

Removes a file (?name=) and its segments from a project.

Send the version read (If-Match or ?version=).

Access: manage on the project.

- Parameters: `id` (path), `name` (query), `If-Match` or `?version=`
- Answer: 200 OK: [ProjectView](#projectview)
- Errors: 401, 403, 404, 409, 428, 422 (see [Error](#error))

### `GET /api/projects/{id}/files/content`

Sends a project file (?name=) as it was uploaded.

Access: read on the project, or work in it.

- Parameters: `id` (path), `name` (query)
- Answer: 200 OK: `application/octet-stream`
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/projects/{id}/files/translated`

Sends a file of the project (?name=) with the translations of a target language (?lang=): a document (Word, PowerPoint, Excel, a web page, InDesign IDML) in the target language, a bilingual file with its translations and states, or a text.

The translation of a PDF is the Word document its text was read into. Segments not translated yet keep their source text.

Access: read on the project, or work in it.

- Parameters: `id` (path), `name` (query), `lang` (query)
- Answer: 200 OK: `application/octet-stream`
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/projects/{id}/issues`

Marks an issue on a segment (?file=, ?lang=, ?segment=): what a reviewer found wrong with its translation.

The body has "category" (accuracy, fluency, terminology, style, locale, formatting or other), "severity" (minor, major or critical: 1, 5 and 25 penalty points) and an optional "note". The segment's translation and version stay as they are. The answer is the segment with its issues.

Access: read on the project, or work in it.

- Parameters: `id` (path), `file` (query), `lang` (query), `segment` (query)
- Body: {`category`: string, `note`: string, `severity`: string}
- Answer: 201 Created: [projectdb.StoredSegment](#projectdbstoredsegment)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `DELETE /api/projects/{id}/issues`

Takes an issue (?issue=) off a segment (?file=, ?lang=, ?segment=).

The answer is the segment with the issues that are left.

Access: whoever marked the issue; manage on the project for anyone's.

- Parameters: `id` (path), `file` (query), `lang` (query), `segment` (query), `issue` (query)
- Answer: 200 OK: [projectdb.StoredSegment](#projectdbstoredsegment)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/projects/{id}/lookup`

Finds matches for a segment in the memories the project looks up in for the language.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Body: [LookupRequest](#lookuprequest)
- Answer: 200 OK: list of [MatchView](#matchview)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/projects/{id}/memories/{name}/export`

Sends a memory the project looks up in, or searches with the concordance, as TMX, to the project's people: the apps match against it while translating.

Memories the project only updates are not sent.

Access: read on the project, or work in it.

- Parameters: `id` (path), `name` (path)
- Answer: 200 OK: `application/x-tmx+xml`
- Errors: 401, 403, 404 (see [Error](#error))

### `GET /api/projects/{id}/memories/{name}/extract`

Sends, as TMX, the units of a memory the project looks up in that match its segments in a language: what the apps need to show matches and analyse the project, without the rest of a large memory.

Concordance searches go to the server.

Access: read on the project, or work in it.

- Parameters: `id` (path), `name` (path), `lang` (query)
- Answer: 200 OK: `application/x-tmx+xml`
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/projects/{id}/presence`

Lists who has a file of the project open in a language (?file=&lang=), and at which segment.

Access: read on the project, or work in it.

- Parameters: `id` (path), `file` (query), `lang` (query)
- Answer: 200 OK: list of [projectdb.Presence](#projectdbpresence)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `PUT /api/projects/{id}/presence`

Says the person is at a segment; others see it until they leave or stop saying so for a while.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Body: {`file`: string, `lang`: string, `segment`: string}
- Answer: 200 OK: [projectdb.Presence](#projectdbpresence)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `DELETE /api/projects/{id}/presence`

Says the person has left the project's files.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/projects/{id}/pretranslate`

Fills a target language's empty segments (or all languages') from the project's memories, as its settings say.

Access: manage on the project.

- Parameters: `id` (path)
- Body: {`lang`: string, `prepare`: [project.Prepare](#projectprepare) or null}
- Answer: 200 OK: list of {`filled`: integer, `lang`: string}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/projects/{id}/progress`

Counts, for each translatable file and target language, the segments and words to translate and how many are confirmed.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Answer: 200 OK: list of [FileProgress](#fileprogress)
- Errors: 401, 403, 404 (see [Error](#error))

### `POST /api/projects/{id}/qa`

Runs the project's quality checks on a file's segments in a target language (or some of them), with translations not saved yet if given, and lists the issues found.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Body: [QaRequest](#qarequest)
- Answer: 200 OK: list of [qa.Issue](#qaissue)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/projects/{id}/qa/report`

Writes the quality report of a file in a target language (?file=&lang=) as HTML, CSV or JSON (?format=, html by default).

In a project with a grammar service the translations are sent to it, and what it finds is in the report.

Access: read on the project, or work in it.

- Parameters: `id` (path), `lang` (query), `file` (query), `format` (query)
- Answer: 200 OK: `application/x-www-form-urlencoded`, `application/octet-stream`
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/projects/{id}/recognize`

Finds the terms of the project's termbases in a source text, with their translations into a target language, as the editor shows them for a segment.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Body: [RecognizeRequest](#recognizerequest)
- Answer: 200 OK: list of [TermHit](#termhit)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/projects/{id}/return-package`

Sends the return package of a project made from a package: a Trados return package (.sdlrpx) or a memoQ delivery package (.mqback) with the package's bilingual files, the project's translations and states in them.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Answer: 200 OK: `application/octet-stream`
- Errors: 401, 403, 404 (see [Error](#error))

### `GET /api/projects/{id}/review`

Is the scored review of the project: for each file and target language (or only ?lang=) the issues marked, by severity and category, the penalty points (1 for a minor issue, 5 for a major one, 25 for a critical one) and the score, 100 less the penalty in percent of the source words; and the same for all together.

With ?format=csv the answer is every issue as a CSV table, with its segment's number, source and translation.

Access: read on the project, or work in it.

- Parameters: `id` (path), `lang` (query), `format` (query)
- Answer: 200 OK: [project.Review](#projectreview)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/projects/{id}/segments`

Returns a file's segments in a target language (?file=&lang=), with their versions.

Access: read on the project, or work in it.

- Parameters: `id` (path), `file` (query), `lang` (query)
- Answer: 200 OK: list of [projectdb.StoredSegment](#projectdbstoredsegment)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `PUT /api/projects/{id}/segments`

Saves a translation.

Editors of the project may save any segment; translators and reviewers those assigned to them. A confirmed translation goes into the memories the project updates.

Access: edit on the project, or the segment's assignee.

- Parameters: `id` (path), `file` (query), `lang` (query), `segment` (query), `If-Match` or `?version=`
- Body: [project.Edit](#projectedit)
- Answer: 200 OK: [projectdb.StoredSegment](#projectdbstoredsegment)
- Errors: 401, 403, 404, 409, 428, 422 (see [Error](#error))

### `POST /api/projects/{id}/segments/join`

Makes one segment of a segment (?file=, ?lang=, ?segment=) and the one after it in the same paragraph (in XLIFF, the same unit of the file), whether they are whole sentences or parts of ones that were split.

Their translations are put together, and the file is written with one translation where they stood. The answer is the file's segments as they are now.

Access: edit on the project, or the assignee of both segments.

- Parameters: `id` (path), `file` (query), `lang` (query), `segment` (query), `If-Match` or `?version=`
- Answer: 200 OK: list of [projectdb.StoredSegment](#projectdbstoredsegment)
- Errors: 401, 403, 404, 409, 428, 422 (see [Error](#error))

### `POST /api/projects/{id}/segments/split`

Cuts a segment (?file=, ?lang=, ?segment=) in two where the body's "before", the start of its source, ends.

The translation stays with the first part. Any segment of a document or of a plain XLIFF file can be split, also one that was joined: split where it was joined, the two stand apart as the file had them. The file is written with the parts' translations one after the other. Only that language of the file is cut differently. Files of Trados and memoQ keep their segments. The answer is the file's segments as they are now.

Access: edit on the project, or the segment's assignee.

- Parameters: `id` (path), `file` (query), `lang` (query), `segment` (query), `If-Match` or `?version=`
- Body: {`before`: [content.Content](#contentcontent)}
- Answer: 200 OK: list of [projectdb.StoredSegment](#projectdbstoredsegment)
- Errors: 401, 403, 404, 409, 428, 422 (see [Error](#error))

### `PUT /api/projects/{id}/settings`

Replaces a project's settings.

They are checked as when the project was made. Send the version read (If-Match or ?version=).

Access: manage on the project.

- Parameters: `id` (path), `If-Match` or `?version=`
- Body: [project.Settings](#projectsettings)
- Answer: 200 OK: [ProjectView](#projectview)
- Errors: 401, 403, 404, 409, 428, 422 (see [Error](#error))

### `GET /api/projects/{id}/terminology`

Returns the termbases a project works with that are on the server: its own (with the masters' entries layered under it) and the masters it uses directly.

The apps find terms in segments with them.

Access: read on the project, or work in it.

- Parameters: `id` (path)
- Answer: 200 OK: list of [TermbaseView](#termbaseview)
- Errors: 401, 403, 404 (see [Error](#error))

### `POST /api/projects/{id}/terms`

Adds a term found while translating where the project's setup puts new terms: its own termbase, or the master in Direct mode.

Access: edit on the project, or work in it.

- Parameters: `id` (path)
- Body: [term.Entry](#termentry)
- Answer: 201 Created: {`entry`: [termdb.Stored](#termdbstored), `termbase`: string}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/templates`

Lists the project templates.

Access: signed in.

- Answer: 200 OK: list of [projectdb.StoredTemplate](#projectdbstoredtemplate)
- Errors: 401, 403 (see [Error](#error))

### `PUT /api/templates`

Makes or replaces a project template.

Send the version read, 0 for a new one (If-Match or ?version=).

Access: those who make projects (manage on all projects) and administrators.

- Parameters: `If-Match` or `?version=`
- Body: [project.Template](#projecttemplate)
- Answer: 200 OK: [projectdb.StoredTemplate](#projectdbstoredtemplate)
- Errors: 401, 403, 409, 428, 422 (see [Error](#error))

### `DELETE /api/templates`

Deletes a template (?name=).

Access: those who make projects and administrators.

- Parameters: `name` (query)
- Answer: 204 No Content
- Errors: 401, 403, 422 (see [Error](#error))

## Files, checks and spelling

### `GET /api/dictionaries`

Lists the spelling dictionaries installed on the server.

Access: signed in.

- Answer: 200 OK: list of [projectdb.Dictionary](#projectdbdictionary)
- Errors: 401, 403 (see [Error](#error))

### `POST /api/dictionaries`

Installs the Hunspell dictionaries in a LibreOffice extension (.oxt) or ZIP file (the request body), replacing those of the same names, and lists them.

With Content-Type application/json the body is one dictionary as its .aff and .dic files, with its name and, if the files do not say it, its language.

Access: those who make projects (manage on all projects) and administrators.

- Parameters: `Content-Type` (header)
- Body: [LooseDictionary](#loosedictionary)
- Answer: 200 OK: list of [projectdb.Dictionary](#projectdbdictionary)
- Errors: 401, 403, 422 (see [Error](#error))

### `DELETE /api/dictionaries/{name}`

Removes a spelling dictionary.

One a project or template spells with is not removed; the answer names them.

Access: those who make projects and administrators.

- Parameters: `name` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/dictionaries/{name}/check`

Checks the spelling of words with a dictionary.

Access: signed in.

- Parameters: `name` (path)
- Body: [SpellRequest](#spellrequest)
- Answer: 200 OK: [SpellResult](#spellresult)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/dictionaries/{name}/suggest`

Suggests corrections for a word (?word=), at most ?max= (5).

Access: signed in.

- Parameters: `name` (path), `max` (query), `word` (query)
- Answer: 200 OK: list of string
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/files/inspect`

Reads a document (the request body, named by ?name=) and says what it is: its format, words and segments, the source language it seems to be in, and for bilingual files and Trados packages their languages and files.

Nothing is kept.

Access: signed in.

- Parameters: `name` (query)
- Body: the file's bytes
- Answer: 200 OK: [Inspection](#inspection)
- Errors: 401, 403, 422 (see [Error](#error))

### `GET /api/qa/kinds`

Lists the quality checks, which of them run by default, and what each finds.

Access: signed in.

- Answer: 200 OK: list of {`default`: boolean, `description`: string, `kind`: [qa.Kind](#qakind), `name`: string}
- Errors: 401, 403 (see [Error](#error))

## Memories

### `GET /api/memories`

Lists the memories the person may read, with their role on each.

Access: signed in; each memory needs read.

- Answer: 200 OK: list of [MemoryView](#memoryview)
- Errors: 401, 403 (see [Error](#error))

### `POST /api/memories`

Makes a memory for one language pair.

The person who makes it manages it.

Access: those who manage memories (a manage grant on all memories) and administrators.

- Body: {`client`: string, `name`: string, `sourceLang`: string, `targetLang`: string}
- Answer: 201 Created: [MemoryView](#memoryview)
- Errors: 401, 403, 422 (see [Error](#error))

### `GET /api/memories/{name}`

Returns a memory's languages, size and the person's role on it.

Access: read on the memory.

- Parameters: `name` (path)
- Answer: 200 OK: [MemoryView](#memoryview)
- Errors: 401, 403, 404 (see [Error](#error))

### `DELETE /api/memories/{name}`

Deletes a memory and the grants on it.

A memory a project or template uses is not deleted; the answer names them.

Access: manage on the memory.

- Parameters: `name` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/memories/{name}/changes`

Lists the memory's changes after ?since= (a change number), oldest first, at most ?limit=.

Access: read on the memory.

- Parameters: `name` (path), `since` (query), `limit` (query)
- Answer: 200 OK: {`changes`: list of [tmdb.Change](#tmdbchange), `more`: boolean}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/memories/{name}/concordance`

Searches the memory's sources (or targets) for a word or phrase.

Access: read on the memory.

- Parameters: `name` (path)
- Body: [ConcordanceRequest](#concordancerequest)
- Answer: 200 OK: list of [ConcordanceHit](#concordancehit)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/memories/{name}/export`

Sends the whole memory as TMX.

Access: read on the memory.

- Parameters: `name` (path)
- Answer: 200 OK: `application/x-tmx+xml`
- Errors: 401, 403, 404 (see [Error](#error))

### `POST /api/memories/{name}/import`

Reads a TMX file (the request body) into a memory.

?policy=overwrite replaces translations of sources that are there; ?variants=exact takes only the memory's exact languages, not their variants (en-US for en-GB).

Access: edit on the memory.

- Parameters: `name` (path), `variants` (query), `policy` (query)
- Body: the file's bytes
- Answer: 200 OK: {`read`: integer, `skipped`: integer, `stored`: integer}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/memories/{name}/lookup`

Finds the memory's matches for a segment, best first.

Access: read on the memory.

- Parameters: `name` (path)
- Body: [LookupRequest](#lookuprequest)
- Answer: 200 OK: list of [MatchView](#matchview)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/memories/{name}/units`

Adds a translation unit.

?policy=keep (the default) keeps other translations of the same source; ?policy=overwrite replaces them.

Access: edit on the memory.

- Parameters: `name` (path), `policy` (query)
- Body: [UnitInput](#unitinput)
- Answer: 201 Created: [UnitView](#unitview)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/memories/{name}/units/batch`

Adds many units to a memory at once, all or none.

?policy= is as for one unit. The answer says how many were added.

Access: edit on the memory.

- Parameters: `name` (path), `policy` (query)
- Body: {`units`: list of [UnitInput](#unitinput)}
- Answer: 201 Created: {`added`: integer}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/memories/{name}/units/{uid}`

Returns a translation unit.

Access: read on the memory.

- Parameters: `name` (path), `uid` (path)
- Answer: 200 OK: [UnitView](#unitview)
- Errors: 401, 403, 404 (see [Error](#error))

### `PUT /api/memories/{name}/units/{uid}`

Changes a translation unit.

Send the version read (If-Match or ?version=).

Access: edit on the memory.

- Parameters: `name` (path), `uid` (path), `If-Match` or `?version=`
- Body: [UnitInput](#unitinput)
- Answer: 200 OK: [UnitView](#unitview)
- Errors: 401, 403, 404, 409, 428, 422 (see [Error](#error))

### `DELETE /api/memories/{name}/units/{uid}`

Deletes a translation unit.

Access: edit on the memory.

- Parameters: `name` (path), `uid` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

## Termbases

### `GET /api/termbases`

Lists the termbases the person may read, with their kind, upstreams and the person's role.

Access: signed in; each termbase needs read (directly or through a project that uses it).

- Answer: 200 OK: list of [TermbaseSummary](#termbasesummary)
- Errors: 401, 403 (see [Error](#error))

### `POST /api/termbases`

Makes a master termbase, or a project termbase layered over upstream masters (whose languages it gets if none are given).

The person who makes it manages it.

Access: those who manage termbases (manage on all termbases) and administrators; read on each upstream.

- Body: [NewTermbase](#newtermbase)
- Answer: 201 Created: [TermbaseDetail](#termbasedetail)
- Errors: 401, 403, 422 (see [Error](#error))

### `GET /api/termbases/{name}`

Returns a termbase's definition (languages, fields, picklists), size and the person's role.

Access: read on the termbase.

- Parameters: `name` (path)
- Answer: 200 OK: [TermbaseDetail](#termbasedetail)
- Errors: 401, 403, 404 (see [Error](#error))

### `DELETE /api/termbases/{name}`

Deletes a termbase and the grants on it.

A termbase a project or template uses is not deleted, and neither is a master that project termbases are based on; the answer names them.

Access: manage on the termbase.

- Parameters: `name` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/termbases/{name}/changes`

Lists the termbase's changes after ?since= (a change number), oldest first, at most ?limit=.

Access: read on the termbase.

- Parameters: `name` (path), `since` (query), `limit` (query)
- Answer: 200 OK: {`changes`: list of [termdb.Change](#termdbchange), `more`: boolean}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `PUT /api/termbases/{name}/definition`

Changes a termbase's languages, fields and picklists.

Removing a language or field that entries use is refused: use the languages and fields routes for that.

Access: manage on the termbase.

- Parameters: `name` (path)
- Body: [term.Definition](#termdefinition)
- Answer: 200 OK: [TermbaseDetail](#termbasedetail)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/termbases/{name}/deleted`

Lists deleted entries.

Access: read on the termbase.

- Parameters: `name` (path)
- Answer: 200 OK: list of [termdb.Deletion](#termdbdeletion)
- Errors: 401, 403, 404 (see [Error](#error))

### `POST /api/termbases/{name}/deleted/{eid}/restore`

Brings a deleted entry back as it was when it was deleted, under its old ID if that is still free, and answers with the entry.

Access: edit on the termbase.

- Parameters: `name` (path), `eid` (path)
- Answer: 201 Created: [termdb.Stored](#termdbstored)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/termbases/{name}/duplicates`

Lists groups of entry IDs that share a term in a language (?lang=), or in any language.

Access: read on the termbase.

- Parameters: `name` (path), `lang` (query)
- Answer: 200 OK: list of list of string
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/termbases/{name}/entries`

Finds entries by text in a language (?text=&lang=) and term status (?status=), a page at a time (?limit=&offset=), with the number found.

Access: read on the termbase.

- Parameters: `name` (path), `limit` (query), `offset` (query), `text` (query), `lang` (query), `status` (query)
- Answer: 200 OK: {`entries`: list of [termdb.Stored](#termdbstored), `total`: integer}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/termbases/{name}/entries`

Adds an entry.

Who made it and when is the server's to say.

Access: edit on the termbase.

- Parameters: `name` (path)
- Body: [EntryWrite](#entrywrite)
- Answer: 201 Created: [termdb.Stored](#termdbstored)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/termbases/{name}/entries/{eid}`

Returns an entry with its version.

Access: read on the termbase.

- Parameters: `name` (path), `eid` (path)
- Answer: 200 OK: [termdb.Stored](#termdbstored)
- Errors: 401, 403, 404 (see [Error](#error))

### `PUT /api/termbases/{name}/entries/{eid}`

Replaces an entry.

Send the version read (If-Match or ?version=).

Access: edit on the termbase.

- Parameters: `name` (path), `eid` (path), `If-Match` or `?version=`
- Body: [EntryWrite](#entrywrite)
- Answer: 200 OK: [termdb.Stored](#termdbstored)
- Errors: 401, 403, 404, 409, 428, 422 (see [Error](#error))

### `DELETE /api/termbases/{name}/entries/{eid}`

Deletes an entry, with an optional ?summary= for its history.

Deleted entries are listed at /deleted.

Access: edit on the termbase.

- Parameters: `name` (path), `eid` (path), `summary` (query)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/termbases/{name}/entries/{eid}/history`

Lists every version of an entry, oldest first.

Access: read on the termbase.

- Parameters: `name` (path), `eid` (path)
- Answer: 200 OK: list of [termdb.Version](#termdbversion)
- Errors: 401, 403, 404 (see [Error](#error))

### `POST /api/termbases/{name}/entries/{eid}/revert`

Sets an entry back to an earlier version, as a new version.

Access: edit on the termbase.

- Parameters: `name` (path), `eid` (path)
- Body: {`version`: integer}
- Answer: 200 OK: [termdb.Stored](#termdbstored)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/termbases/{name}/export`

Sends the termbase as a file: ?format=tbx (the default), tbx2, multiterm, csv, tsv or xlsx.

Access: read on the termbase.

- Parameters: `name` (path), `format` (query)
- Answer: 200 OK: `application/octet-stream`
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/termbases/{name}/export`

Sends chosen entries of the termbase as a file, in the formats of the export: those a search shows, for one.

Access: read on the termbase.

- Parameters: `name` (path), `format` (query)
- Body: {`entries`: list of string}
- Answer: 200 OK: `application/octet-stream`
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `PUT /api/termbases/{name}/fields/{field}`

Renames a field in the definition and in every entry, and answers how many entries changed.

Access: manage on the termbase.

- Parameters: `name` (path), `field` (path)
- Body: {`name`: string}
- Answer: 200 OK: map of integer
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `DELETE /api/termbases/{name}/fields/{field}`

Removes a field from the definition and from every entry, and answers how many entries changed.

Access: manage on the termbase.

- Parameters: `name` (path), `field` (path)
- Answer: 200 OK: map of integer
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/termbases/{name}/import`

Reads a termbase file (the request body: TBX, MultiTerm XML, a MultiTerm termbase file (.sdltb), CSV, TSV or Excel) into the termbase.

?match=term|id|none and ?onMatch=merge|overwrite|skip|add say what happens to entries that are there already; ?dryRun=1 reports without changing anything, with the first lines of what would happen. ?mapping= (JSON) says what a spreadsheet's columns hold, ?languages= (a JSON object) which termbase language each language of the file goes under, with "" to leave one out, and ?status=approved|proposed which status terms without one get.

Access: edit on the termbase.

- Parameters: `name` (path), `filename` (query), `format` (query), `mapping` (query), `languages` (query), `status` (query), `match` (query), `lang` (query), `onMatch` (query), `dryRun` (query)
- Body: the file's bytes
- Answer: 200 OK: {`actions`: integer, `added`: integer, `dropped`: integer, `fields`: list of string, `imported`: boolean, `invalid`: integer, `languages`: list of string, `merged`: integer, `overwritten`: integer, `plan`: list of [PlannedEntry](#plannedentry), `read`: [term.Report](#termreport), `skipped`: integer, `unchanged`: integer, `warnings`: list of string}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `DELETE /api/termbases/{name}/languages/{lang}`

Removes a language from the definition and from every entry, and answers how many entries changed.

Access: manage on the termbase.

- Parameters: `name` (path), `lang` (path)
- Answer: 200 OK: map of integer
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/termbases/{name}/layered`

Returns a project termbase's entries with its masters' entries layered under them: overrides replace, removals hide.

Access: read on the termbase.

- Parameters: `name` (path)
- Answer: 200 OK: list of [termdb.Layered](#termdblayered)
- Errors: 401, 403, 404 (see [Error](#error))

### `GET /api/termbases/{name}/lookup`

Finds the entries that have a term (?text=), case and spacing aside, in a language or its variants (?lang=; empty for all).

Access: read on the termbase.

- Parameters: `name` (path), `text` (query), `lang` (query)
- Answer: 200 OK: list of [termdb.Stored](#termdbstored)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/termbases/{name}/merge`

Merges entries into the one kept, and deletes the others.

Access: edit on the termbase.

- Parameters: `name` (path)
- Body: {`keep`: string, `others`: list of string}
- Answer: 200 OK: [termdb.Stored](#termdbstored)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/termbases/{name}/merge-requests`

Lists a project termbase's merge requests, or a master's incoming ones, newest first.

Access: read on the termbase.

- Parameters: `name` (path)
- Answer: 200 OK: list of [termdb.MergeRequest](#termdbmergerequest)
- Errors: 401, 403, 404 (see [Error](#error))

### `POST /api/termbases/{name}/merge-requests`

Submits a project termbase's changes to one of its masters for review.

It takes editing the project termbase and suggesting in the master.

Access: edit on the project termbase and suggest on the master.

- Parameters: `name` (path)
- Body: {`deadline`: string (date-time), `master`: string, `title`: string}
- Answer: 201 Created: [termdb.MergeRequest](#termdbmergerequest)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `PUT /api/termbases/{name}/name`

Renames a termbase, and the grants, projects and templates that refer to it.

Access: manage on the termbase.

- Parameters: `name` (path)
- Body: {`name`: string}
- Answer: 200 OK: [TermbaseDetail](#termbasedetail)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/termbases/{name}/overrides`

Copies a master entry into the project termbase, to change it there.

Access: edit on the project termbase.

- Parameters: `name` (path)
- Body: [MasterEntry](#masterentry)
- Answer: 201 Created: [termdb.Stored](#termdbstored)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/termbases/{name}/removals`

Lists the master entries hidden in a project termbase.

Access: read on the termbase.

- Parameters: `name` (path)
- Answer: 200 OK: list of [termdb.Removal](#termdbremoval)
- Errors: 401, 403, 404 (see [Error](#error))

### `POST /api/termbases/{name}/removals`

Hides a master entry in the project; merging asks the master's reviewers to delete it there.

Access: edit on the project termbase.

- Parameters: `name` (path)
- Body: [MasterEntry](#masterentry)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `DELETE /api/termbases/{name}/removals`

Takes a removal back (?master=&entry=): the master entry shows in the project again, and merging no longer asks to delete it.

Access: edit on the project termbase.

- Parameters: `name` (path), `master` (query), `entry` (query)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/termbases/{name}/review-rules`

Returns how merge requests into the termbase are reviewed.

Access: read on the termbase.

- Parameters: `name` (path)
- Answer: 200 OK: [termdb.ReviewRules](#termdbreviewrules)
- Errors: 401, 403, 404 (see [Error](#error))

### `PUT /api/termbases/{name}/review-rules`

Sets how merge requests into the termbase are reviewed.

Access: manage on the termbase.

- Parameters: `name` (path)
- Body: [termdb.ReviewRules](#termdbreviewrules)
- Answer: 200 OK: [termdb.ReviewRules](#termdbreviewrules)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `PUT /api/termbases/{name}/upstreams`

Sets the masters a project termbase is layered over.

Access: manage on the termbase; read on each upstream.

- Parameters: `name` (path)
- Body: {`upstreams`: list of string}
- Answer: 200 OK: [TermbaseDetail](#termbasedetail)
- Errors: 401, 403, 404, 422 (see [Error](#error))

## Merge requests

### `GET /api/merge-requests/{id}`

Returns a merge request.

Access: read on its master.

- Parameters: `id` (path)
- Answer: 200 OK: [termdb.MergeRequest](#termdbmergerequest)
- Errors: 401, 403, 404 (see [Error](#error))

### `POST /api/merge-requests/{id}/close`

Closes a merge request; items not decided come back in the next one.

Access: approve on the master.

- Parameters: `id` (path)
- Answer: 200 OK: [termdb.MergeRequest](#termdbmergerequest)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/merge-requests/{id}/decide`

Records a decision on the items a filter selects: approved, approved-with-edits, changes-requested, rejected or postponed.

Access: approve on the master.

- Parameters: `id` (path)
- Body: {`decision`: [termdb.Decision](#termdbdecision), `filter`: [termdb.Filter](#termdbfilter)}
- Answer: 200 OK: [termdb.DecideResult](#termdbdecideresult)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/merge-requests/{id}/items`

Lists the items with a simple query: ?state=, ?reviewer=, ?sort=, ?limit=, ?offset=.

Access: read on the master.

- Parameters: `id` (path), `reviewer` (query), `sort` (query), `state` (query), `limit` (query), `offset` (query)
- Answer: 200 OK: {`items`: list of [termdb.Item](#termdbitem), `total`: integer}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/merge-requests/{id}/items`

Lists the items a full query (filter, frequencies) selects.

Access: read on the master.

- Parameters: `id` (path)
- Body: [ItemQuery](#itemquery)
- Answer: 200 OK: {`items`: list of [termdb.Item](#termdbitem), `total`: integer}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/merge-requests/{id}/items/{iid}/revert`

Undoes the decision on an item of an open merge request.

Access: approve on the master.

- Parameters: `id` (path), `iid` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/merge-requests/{id}/progress`

Counts a merge request's items by state and by reviewer.

Access: read on the master.

- Parameters: `id` (path)
- Answer: 200 OK: [termdb.Progress](#termdbprogress)
- Errors: 401, 403, 404 (see [Error](#error))

### `GET /api/merge-requests/{id}/reviewers`

Lists the reviewers given part of a merge request.

Access: read on the master.

- Parameters: `id` (path)
- Answer: 200 OK: list of [termdb.Assignment](#termdbassignment)
- Errors: 401, 403, 404 (see [Error](#error))

### `PUT /api/merge-requests/{id}/reviewers`

Gives a reviewer (a username) part of the request.

The reviewer must be able to approve in the master.

Access: manage on the master.

- Parameters: `id` (path)
- Body: [termdb.Assignment](#termdbassignment)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `DELETE /api/merge-requests/{id}/reviewers`

Takes a reviewer (?reviewer=, a username) off a merge request.

Access: manage on the master.

- Parameters: `id` (path), `reviewer` (query)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/merge-requests/{id}/sample`

Picks up to n waiting items at random from those the filter selects, for a spot check of a large request.

The same seed gives the same sample.

Access: read on the master.

- Parameters: `id` (path)
- Body: {`filter`: [termdb.Filter](#termdbfilter), `n`: integer, `seed`: integer}
- Answer: 200 OK: list of [termdb.Item](#termdbitem)
- Errors: 401, 403, 404, 422 (see [Error](#error))

## Live feed

### `GET /api/events`

Streams the live feed: the changes the person may see in projects, memories, termbases and their account, and presence in projects, as text/event-stream.

Reconnect with Last-Event-ID (or ?since=) to get what was missed. A "signedOut" event ends the stream when the sign-in ends.

Access: signed in; each event is sent only to those who may see it.

- Parameters: `since` (query), `Last-Event-ID` (header)
- Answer: 200 OK: `text/event-stream`
- Errors: 401, 403, 422 (see [Error](#error))

## Other

### `POST /api/align`

Pairs the segments of a document with those of its translation, so that translations done before can be put into a memory (POST /api/memories/{name}/units/batch).

The two documents may be of different formats. The answer has the segments of both documents and the pairs made of them; a review joins and splits them otherwise. Sent again with the pairs the review fixed, the rest is paired around them. Nothing is kept.

Access: signed in.

- Body: [AlignRequest](#alignrequest)
- Answer: 200 OK: [align.Result](#alignresult)
- Errors: 401, 403, 422 (see [Error](#error))

### `GET /api/providers`

Lists the services: the translation services the editor can ask for a suggestion and the grammar services that read translations.

Administrators also see each one's address and the name of its header; the header's value is never given out.

Access: signed in.

- Answer: 200 OK: list of [ProviderView](#providerview)
- Errors: 401, 403 (see [Error](#error))

### `PUT /api/providers/{name}`

Adds a service or changes the one of the name: its kind ("translate", the default, or "check" for a grammar service), its address, and a header sent with each request to sign it.

Without headerValue, the service keeps the value it has.

Access: administrators.

- Parameters: `name` (path)
- Body: {`headerName`: string, `headerValue`: string or null, `kind`: string, `url`: string}
- Answer: 200 OK: [ProviderView](#providerview)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `DELETE /api/providers/{name}`

Removes a service.

Access: administrators.

- Parameters: `name` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/providers/{name}/check`

Asks a grammar service what it finds in a text of a language.

The text is sent to the service's address, outside the server. Spelling is not asked for.

Access: signed in.

- Parameters: `name` (path)
- Body: {`lang`: string, `text`: string}
- Answer: 200 OK: {`matches`: list of [GrammarMatch](#grammarmatch)}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/providers/{name}/translate`

Asks a translation service for the translations of segments.

The texts are sent to the service's address, outside the server; nothing is sent to a service that is not asked. A translation that does not keep the source's tags comes back without tags, marked.

Access: signed in.

- Parameters: `name` (path)
- Body: [ProviderRequest](#providerrequest)
- Answer: 200 OK: {`translations`: list of [ProviderTranslation](#providertranslation)}
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `GET /api/roles`

Lists the roles: the five that come with the server (read, suggest, edit, approve, manage, each a permission and the ones before it) and those the administrators defined, each with its permissions and how many grants use it.

Access: signed in.

- Answer: 200 OK: list of [authdb.RoleDef](#authdbroledef)
- Errors: 401, 403 (see [Error](#error))

### `PUT /api/roles/{name}`

Defines a role as a set of permissions, or changes or renames the one of that name.

A change takes effect at once for everyone who has the role. The five roles that come with the server are not changed.

Access: administrators.

- Parameters: `name` (path)
- Body: [RoleRequest](#rolerequest)
- Answer: 200 OK: [authdb.RoleDef](#authdbroledef)
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `DELETE /api/roles/{name}`

Removes a role the administrators defined.

A role people have is not removed: give them another first.

Access: administrators.

- Parameters: `name` (path)
- Answer: 204 No Content
- Errors: 401, 403, 404, 422 (see [Error](#error))

### `POST /api/termfiles/inspect`

Reads a termbase file (the request body, named by ?filename=) and says what it holds: its format, entries, terms, languages and fields, and for spreadsheets the first rows with what each column is taken for.

?mapping= (JSON) reads a spreadsheet with a column mapping of your own, and ?languages= (a JSON object) says which language each of the file's goes under, as for an import. Nothing is kept.

Access: signed in.

- Parameters: `format` (query), `filename` (query), `mapping` (query), `languages` (query)
- Body: the file's bytes
- Answer: 200 OK: [TermFileInspection](#termfileinspection)
- Errors: 401, 403, 422 (see [Error](#error))

## Types

### Access

access is what someone may do in a project.

| Field | Type | Description |
|---|---|---|
| `assigned` | boolean | Assigned is set when they have work in the project, which lets them see it without a grant. |
| `role` | [authdb.Role](#authdbrole) |  |

### AlignRequest

| Field | Type | Description |
|---|---|---|
| `fixed` | list of [align.Fixed](#alignfixed) | Fixed are pairs made or checked by hand, by the positions of their segments in an earlier answer for the same documents: they stay, and the rest is paired again around them. |
| `source` | [AlignedFile](#alignedfile) |  |
| `target` | [AlignedFile](#alignedfile) |  |

### AlignedFile

alignedFile is one of the two documents of an alignment: its name, which tells the format, its language and its bytes in base64.

| Field | Type | Description |
|---|---|---|
| `data` | string (byte) |  |
| `lang` | string |  |
| `name` | string |  |

### AnalysisRequest

| Field | Type | Description |
|---|---|---|
| `file` | string | File is the one file to analyse; empty for all. The editor asks for the file it has open, to show each segment's best match. |
| `lang` | string | Lang is the target language; empty for all. |
| `weights` | map of number | Weights are the share of the full rate charged per band, in percent; with them the report has weighted words. |

### BilingualInfo

| Field | Type | Description |
|---|---|---|
| `format` | string | Format is xliff-1.2, xliff-2, sdlxliff or mqxliff. |
| `sourceLang` | string |  |
| `targetLang` | string |  |
| `translated` | integer | Translated counts the segments the file has translations for. |

### CatalogMemory

| Field | Type | Description |
|---|---|---|
| `name` | string |  |
| `role` | string |  |
| `sourceLang` | string |  |
| `targetLang` | string |  |
| `units` | integer |  |

### CatalogTermbase

| Field | Type | Description |
|---|---|---|
| `entries` | integer |  |
| `kind` | string | Kind is master or project. |
| `languages` | list of string |  |
| `name` | string |  |
| `role` | string |  |

### ConcordanceHit

| Field | Type | Description |
|---|---|---|
| `memory` | string |  |
| `unit` | [UnitView](#unitview) |  |

### ConcordanceRequest

| Field | Type | Description |
|---|---|---|
| `inTarget` | boolean |  |
| `lang` | string |  |
| `limit` | integer |  |
| `query` | string |  |

### CreateProjectRequest

| Field | Type | Description |
|---|---|---|
| `files` | list of [FileUpload](#fileupload) |  |
| `memories` | list of [tm.Info](#tminfo) | Memories are new memories to make with the project (its own memories); the settings refer to them by name. |
| `settings` | [project.Settings](#projectsettings) |  |
| `termbase` | [term.Definition](#termdefinition) | Termbase makes the project termbase on the server (Layered or Own mode with location server): layered over the settings' masters, or on its own. Its name goes into the settings. |

### EntryWrite

| Field | Type | Description |
|---|---|---|
| `entry` | [term.Entry](#termentry) |  |
| `summary` | string |  |

### Error

apiError is the body of every error answer.

| Field | Type | Description |
|---|---|---|
| `code` | string | Code is stable for the apps: badRequest, signIn, credentials, forbidden, notFound, exists, conflict, invalid, badFile, inUse, locked, closed, decided, needsResolution, notProject, notOnServer, lastAdmin, tooMany, tooLarge, server. |
| `message` | string | Message says what went wrong, for people. |
| `problems` | list of [project.Problem](#projectproblem) | Problems are the problems of a refused request, by field. |

### FileProgress

fileProgress is how far a file is in one target language.

| Field | Type | Description |
|---|---|---|
| `confirmed` | integer |  |
| `confirmedWords` | integer |  |
| `draft` | integer |  |
| `file` | string |  |
| `lang` | string |  |
| `segments` | integer | Segments and Words count what is to translate (not locked); Confirmed and ConfirmedWords what is confirmed, Draft what has a translation not confirmed yet. |
| `words` | integer |  |

### FileUpload

fileUpload is a file as the apps send it: the original bytes (base64 in JSON) and, for a file to translate, its segments per target language as the app's filters read them. fileUpload is a file sent to a project.

| Field | Type | Description |
|---|---|---|
| `data` | string (byte) | Data is the file's bytes. |
| `format` | string | Format is docx, xliff or txt for a file to translate (the server takes it from the name when it is left out), the extension for a reference file. |
| `name` | string | Name is the file's name, with the folder it came from if any. |
| `role` | [project.Role](#projectrole) | Role is translatable, reference, or package for a Trados project package, whose bilingual files the project gets. |
| `segments` | map of list of [project.Segment](#projectsegment) | Segments are a translatable file's segments per target language, as the app read them; left out, the server makes them. |
| `targetLang` | string | TargetLang is a bilingual file's target language; the server reads it from the file when it is left out. |

### GrammarMatch

grammarMatch is something a grammar service found in a text.

| Field | Type | Description |
|---|---|---|
| `category` | string |  |
| `found` | string | Found is the text there. |
| `length` | integer |  |
| `message` | string |  |
| `offset` | integer | Offset and Length say where, in UTF-16 code units of the text. |
| `replacements` | list of string |  |
| `rule` | string | Rule and Category name what the service checked, in its words. |

### GrantView

grantView is a grant with the person's name, for lists.

| Field | Type | Description |
|---|---|---|
| `kind` | [authdb.Kind](#authdbkind) |  |
| `name` | string |  |
| `resource` | string |  |
| `role` | [authdb.Role](#authdbrole) |  |
| `userId` | integer |  |
| `username` | string |  |

### Inspection

inspection is what the server finds in a document before a project is made from it.

| Field | Type | Description |
|---|---|---|
| `bilingual` | [BilingualInfo](#bilingualinfo) | Bilingual is set for an XLIFF file: its variant and languages. |
| `detection` | [langdetect.Detection](#langdetectdetection) | Detection guesses the source language from the text. |
| `format` | string | Format is docx, pdf, xliff, package (a Trados project package), txt and so on, or other for a file that is not translated (a reference file). |
| `name` | string |  |
| `package` | [PackageInfo](#packageinfo) | Package describes a Trados project package and its files. |
| `problem` | string | Problem says why a file that looks translatable can't be read. |
| `segments` | integer |  |
| `translatable` | boolean | Translatable is false for other files and files that can't be read. |
| `warnings` | list of string |  |
| `words` | integer |  |

### ItemQuery

| Field | Type | Description |
|---|---|---|
| `filter` | [termdb.Filter](#termdbfilter) |  |
| `frequency` | map of integer |  |
| `limit` | integer |  |
| `offset` | integer |  |
| `reviewer` | string |  |
| `sort` | string |  |

### LookupRequest

| Field | Type | Description |
|---|---|---|
| `lang` | string |  |
| `limit` | integer |  |
| `minScore` | integer |  |
| `source` | [content.Content](#contentcontent) |  |

### LooseDictionary

looseDictionary is a Hunspell dictionary sent as its two files.

| Field | Type | Description |
|---|---|---|
| `aff` | string (byte) | Aff and Dic are the bytes of the .aff and the .dic file. |
| `dic` | string (byte) |  |
| `lang` | string | Lang is the language it spells. Left out, the .aff file's LANG line or the name says it. |
| `name` | string | Name identifies it; the files' base name ("de_DE_frami"). |

### MasterEntry

| Field | Type | Description |
|---|---|---|
| `entry` | string |  |
| `master` | string |  |

### MatchView

| Field | Type | Description |
|---|---|---|
| `memory` | string |  |
| `score` | integer |  |
| `unit` | [UnitView](#unitview) |  |

### MeResult

| Field | Type | Description |
|---|---|---|
| `grants` | list of [authdb.Grant](#authdbgrant) |  |
| `identities` | list of [authdb.Identity](#authdbidentity) |  |
| `user` | [authdb.User](#authdbuser) |  |

### MemoryView

| Field | Type | Description |
|---|---|---|
| `client` | string | Client is who the memory is kept for, if anyone. |
| `createdAt` | string (date-time) |  |
| `name` | string |  |
| `role` | string |  |
| `sourceLang` | string |  |
| `targetLang` | string |  |
| `units` | integer |  |

### NewTermbase

| Field | Type | Description |
|---|---|---|
| `definition` | [term.Definition](#termdefinition) |  |
| `upstreams` | list of string | Upstreams make it a project termbase layered over these masters. |

### NewUserRequest

| Field | Type | Description |
|---|---|---|
| `admin` | boolean |  |
| `email` | string |  |
| `name` | string |  |
| `password` | string | Password may be empty for someone who signs in only through the identity provider; link their identity afterwards. |
| `username` | string |  |

### OidcStartRequest

| Field | Type | Description |
|---|---|---|
| `device` | string |  |
| `return` | string | Return is where the browser goes afterwards. |

### PackageFile

| Field | Type | Description |
|---|---|---|
| `lang` | string |  |
| `path` | string |  |
| `problem` | string |  |
| `segments` | integer |  |
| `words` | integer |  |

### PackageInfo

| Field | Type | Description |
|---|---|---|
| `files` | list of [PackageFile](#packagefile) |  |
| `name` | string |  |
| `other` | integer | Other counts the package's other entries (memories, termbases), which stay in it. |
| `sourceLang` | string |  |
| `targetLangs` | list of string |  |

### PasswordSignin

| Field | Type | Description |
|---|---|---|
| `device` | string |  |
| `password` | string |  |
| `username` | string |  |

### Person

person is what everyone signed in may see of an account, to choose whom work goes to.

| Field | Type | Description |
|---|---|---|
| `id` | integer |  |
| `name` | string |  |
| `username` | string |  |

### PlannedEntry

plannedEntry is a line of an import's plan.

| Field | Type | Description |
|---|---|---|
| `entry` | string | Entry names the file's entry by its first terms, Existing the entry it meets in the termbase. |
| `existing` | string |  |
| `kind` | string | Kind is add, merge, overwrite or skip. |

### ProjectPreview

projectPreview is what a project would be in one target language.

| Field | Type | Description |
|---|---|---|
| `confirmed` | integer |  |
| `filled` | integer | Filled is how many segments pre-translation would fill with the setup's preparation, and Confirmed how many of those it would confirm. Both are 0 when the setup does not pre-translate. |
| `lang` | string |  |
| `memories` | list of string | Memories are the memories the analysis looked up in, in the setup's order. Memories made with the project are empty and not among them. |
| `report` | [analysis.Report](#analysisreport) | Report is the analysis of the files translated into the language. |

### ProjectView

| Field | Type | Description |
|---|---|---|
| `access` | [Access](#access) |  |
| `changedAt` | string (date-time) |  |
| `changedBy` | string |  |
| `createdAt` | string (date-time) |  |
| `createdBy` | string |  |
| `id` | integer |  |
| `settings` | [project.Settings](#projectsettings) |  |
| `version` | integer | Version counts the changes to the settings and the files. |

### ProviderRequest

| Field | Type | Description |
|---|---|---|
| `segments` | list of {`id`: string, `source`: [content.Content](#contentcontent)} |  |
| `sourceLang` | string |  |
| `targetLang` | string |  |

### ProviderTranslation

| Field | Type | Description |
|---|---|---|
| `id` | string |  |
| `tagsLost` | boolean | TagsLost is set where the service's translation did not have the source's tags as they are, and was taken without tags. |
| `target` | [content.Content](#contentcontent) |  |

### ProviderView

| Field | Type | Description |
|---|---|---|
| `headerName` | string |  |
| `host` | string | Host is where the service is, without the rest of its address. |
| `kind` | string | Kind is "translate" for a translation service and "check" for a grammar service. |
| `name` | string |  |
| `signed` | boolean |  |
| `url` | string | URL, HeaderName and Signed (a header's value is kept) are for administrators. |

### QaRequest

| Field | Type | Description |
|---|---|---|
| `file` | string |  |
| `lang` | string |  |
| `segments` | list of string | Segments are the segments to check, by ID; empty for all. |
| `targets` | map of [content.Content](#contentcontent) | Targets check translations not saved yet, by segment ID. |

### RecognizeRequest

| Field | Type | Description |
|---|---|---|
| `lang` | string | Lang is the target language the entries' translations are for. |
| `text` | string |  |

### RoleRequest

roleRequest defines a role.

| Field | Type | Description |
|---|---|---|
| `name` | string | Name renames the role; leave it out to keep the name in the address. |
| `permissions` | list of [authdb.Role](#authdbrole) | Permissions are what the role allows, of read, suggest, edit, approve and manage; read is always among them. |

### ServerInfo

| Field | Type | Description |
|---|---|---|
| `name` | string |  |
| `provider` | string | Provider names the identity provider people can sign in with. |
| `setup` | boolean | Setup is true while the server has no accounts and the first administrator can be made with the setup token. |
| `version` | string |  |

### SetupRequest

| Field | Type | Description |
|---|---|---|
| `device` | string |  |
| `email` | string |  |
| `name` | string |  |
| `password` | string |  |
| `setupToken` | string |  |
| `username` | string |  |

### SigninResult

signinResult is the answer to a sign-in.

| Field | Type | Description |
|---|---|---|
| `expiresAt` | string (date-time) |  |
| `token` | string | Token goes into the Authorization header of every request. It is shown once, and not at all to a sign-in that asked for the cookie. |
| `user` | [authdb.User](#authdbuser) |  |

### SpellRequest

| Field | Type | Description |
|---|---|---|
| `words` | list of string |  |

### SpellResult

| Field | Type | Description |
|---|---|---|
| `correct` | list of boolean | Correct says, for each word, whether it is spelled right. |

### TermFileInspection

termFileInspection says what a termbase file holds.

| Field | Type | Description |
|---|---|---|
| `dropped` | integer | Dropped counts the entries ?languages= leaves without a term, and Sample names the first entries that stay, by their first terms. |
| `fields` | list of [term.Field](#termfield) |  |
| `format` | [term.Format](#termformat) |  |
| `languages` | list of string |  |
| `mapping` | [term.Mapping](#termmapping) |  |
| `name` | string | Name is the termbase's name in the file, if it has one. |
| `read` | [term.Report](#termreport) |  |
| `rows` | list of list of string | Rows are the first rows of a spreadsheet, Total how many it has, and Mapping what each column was read as: the guess, or the one sent. |
| `sample` | list of string |  |
| `tabular` | boolean | Tabular is set for spreadsheets, whose columns need a mapping. |
| `total` | integer |  |

### TermHit

termHit is a term found in a text, with its place in UTF-16 code units as well, for apps whose strings count those.

| Field | Type | Description |
|---|---|---|
| `end` | integer |  |
| `end16` | integer |  |
| `entryId` | string |  |
| `exact` | boolean | Exact is set when the text has the term as written (case aside). |
| `source` | string | Source is the label the entry was added with. |
| `start` | integer | Start and End are byte offsets into the text. |
| `start16` | integer |  |
| `targets` | list of [term.Term](#termterm) | Targets are the entry's terms in the target language, approved first. |
| `term` | [term.Term](#termterm) | Term is the source-language term that matched. |

### TermbaseDetail

| Field | Type | Description |
|---|---|---|
| `definition` | [term.Definition](#termdefinition) |  |
| `entries` | integer |  |
| `kind` | string | Kind is "master" or "project". |
| `name` | string |  |
| `projects` | list of string | Projects are the project termbases layered over a master, by name. A master is not deleted while it has any. |
| `role` | string |  |
| `terms` | integer |  |
| `upstreams` | list of string | Upstreams are the masters a project is layered over, highest priority first. |

### TermbaseSummary

| Field | Type | Description |
|---|---|---|
| `createdAt` | string (date-time) |  |
| `entries` | integer |  |
| `fields` | list of [term.Field](#termfield) |  |
| `kind` | string |  |
| `languages` | list of string |  |
| `name` | string |  |
| `role` | string |  |
| `terms` | integer |  |
| `upstreams` | list of string |  |

### TermbaseView

termbaseView is a termbase with the entries a project sees in it.

| Field | Type | Description |
|---|---|---|
| `definition` | [term.Definition](#termdefinition) |  |
| `entries` | list of [termdb.Layered](#termdblayered) |  |
| `kind` | string |  |
| `name` | string |  |
| `upstreams` | list of string |  |

### UnitInput

unitInput is a unit as the apps send it.

| Field | Type | Description |
|---|---|---|
| `attributes` | map of string |  |
| `notes` | list of string |  |
| `source` | [content.Content](#contentcontent) |  |
| `target` | [content.Content](#contentcontent) |  |

### UnitView

unitView is a memory unit as the API shows it.

| Field | Type | Description |
|---|---|---|
| `attributes` | map of string |  |
| `changedAt` | string |  |
| `changedBy` | string |  |
| `createdAt` | string |  |
| `createdBy` | string |  |
| `id` | integer |  |
| `notes` | list of string |  |
| `source` | [content.Content](#contentcontent) |  |
| `target` | [content.Content](#contentcontent) |  |
| `usageCount` | integer |  |
| `version` | integer |  |

### UserPatch

| Field | Type | Description |
|---|---|---|
| `admin` | boolean or null |  |
| `disabled` | boolean or null |  |
| `email` | string or null |  |
| `name` | string or null |  |

### align.Fixed

Fixed is a pair someone made or checked by hand: these segments of the document belong to these of the translation, whatever their lengths say.

| Field | Type | Description |
|---|---|---|
| `source` | list of integer |  |
| `target` | list of integer |  |

### align.Pair

Pair is a stretch of the source with its translation. A stretch nothing was found for has an empty other side.

| Field | Type | Description |
|---|---|---|
| `fixed` | boolean | Fixed marks a pair that was given as fixed when pairing again. |
| `score` | integer | Score says how well the two sides fit, from 0 to 100: their lengths and their numbers. Pairs below about 50 deserve a look. |
| `source` | [content.Content](#contentcontent) |  |
| `sourceSegments` | list of integer | SourceSegments and TargetSegments are the segments joined on each side, by their position in the documents. |
| `target` | [content.Content](#contentcontent) |  |
| `targetSegments` | list of integer |  |

### align.Result

Result is an alignment with the segments it was made of, for a review that joins and splits them otherwise.

| Field | Type | Description |
|---|---|---|
| `pairs` | list of [align.Pair](#alignpair) | Pairs are the segments of the document with their translations, in the order of the documents. A pair with an empty side is a segment nothing was found for; a score under 50 deserves a look. |
| `source` | list of [content.Content](#contentcontent) | Source and Target are the segments of the two documents, which the pairs name by their position. |
| `target` | list of [content.Content](#contentcontent) |  |

### analysis.Band

Band is a row of the analysis.

One of `100`, `50`, `75`, `85`, `95`, `context`, `cross-file`, `locked`, `new`, `repetition`.

### analysis.ContextRule

ContextRule says which neighbours make a 100% match a context match: the segment before (Trados) or the segments before and after (memoQ).

One of `both`, `previous`.

### analysis.Counts

Counts are the amounts of a band, a file or a project.

| Field | Type | Description |
|---|---|---|
| `characters` | integer | Characters are counted without spaces. |
| `placeables` | integer | Placeables are numbers, which translators usually copy unchanged. |
| `segments` | integer |  |
| `tags` | integer |  |
| `words` | integer |  |

### analysis.FileReport

FileReport is the analysis of one file.

| Field | Type | Description |
|---|---|---|
| `name` | string |  |
| `rows` | list of [analysis.Row](#analysisrow) |  |
| `total` | [analysis.Counts](#analysiscounts) |  |
| `weighted` | number |  |

### analysis.Report

Report is the analysis of a project.

| Field | Type | Description |
|---|---|---|
| `context` | [analysis.ContextRule](#analysiscontextrule) |  |
| `created` | string (date-time) |  |
| `files` | list of [analysis.FileReport](#analysisfilereport) |  |
| `memories` | list of string |  |
| `rows` | list of [analysis.Row](#analysisrow) |  |
| `segments` | list of [analysis.SegmentResult](#analysissegmentresult) |  |
| `sourceLang` | string |  |
| `targetLang` | string |  |
| `total` | [analysis.Counts](#analysiscounts) |  |
| `weighted` | number |  |
| `weights` | map of number |  |

### analysis.Row

Row is the counts of one band.

| Field | Type | Description |
|---|---|---|
| `band` | [analysis.Band](#analysisband) |  |
| `characters` | integer | Characters are counted without spaces. |
| `name` | string |  |
| `placeables` | integer | Placeables are numbers, which translators usually copy unchanged. |
| `segments` | integer |  |
| `tags` | integer |  |
| `weighted` | number | Weighted is Words times the band's weight, with Options.Weights. |
| `words` | integer |  |

### analysis.SegmentResult

SegmentResult says how a segment was counted.

| Field | Type | Description |
|---|---|---|
| `band` | [analysis.Band](#analysisband) |  |
| `counted` | [analysis.Counts](#analysiscounts) |  |
| `file` | string |  |
| `id` | string |  |
| `memory` | string |  |
| `score` | integer |  |

### authdb.Grant

Grant gives a user a role on a resource.

| Field | Type | Description |
|---|---|---|
| `kind` | [authdb.Kind](#authdbkind) |  |
| `resource` | string |  |
| `role` | [authdb.Role](#authdbrole) |  |
| `userId` | integer |  |

### authdb.Identity

Identity links an account to a subject at an OpenID Connect provider.

| Field | Type | Description |
|---|---|---|
| `issuer` | string |  |
| `subject` | string |  |
| `userId` | integer |  |

### authdb.Kind

Kind is the kind of resource a grant is on.

One of `all`, `memory`, `project`, `termbase`.

### authdb.Role

Role is what someone may do with a resource.

One of `approve`, `edit`, `manage`, `read`, `suggest`.

### authdb.RoleDef

RoleDef is a role as it is defined: its name and what it allows.

| Field | Type | Description |
|---|---|---|
| `builtIn` | boolean | BuiltIn roles come with the server and cannot be changed. |
| `grants` | integer | Grants counts the grants that use the role. |
| `name` | string |  |
| `permissions` | list of [authdb.Role](#authdbrole) |  |

### authdb.Token

Token is a sign-in of a user on a device. Its secret is shown once, when it is issued; the store keeps only a hash.

| Field | Type | Description |
|---|---|---|
| `createdAt` | string (date-time) |  |
| `expiresAt` | string (date-time) |  |
| `id` | integer |  |
| `lastUsed` | string (date-time) |  |
| `name` | string |  |
| `userId` | integer |  |

### authdb.User

User is an account.

| Field | Type | Description |
|---|---|---|
| `admin` | boolean |  |
| `createdAt` | string (date-time) |  |
| `disabled` | boolean |  |
| `email` | string |  |
| `hasPassword` | boolean | HasPassword is false for accounts that sign in only through an identity provider. |
| `id` | integer |  |
| `name` | string |  |
| `username` | string |  |
| `version` | integer |  |

### content.Content

Content is an ordered sequence of runs.

list of [content.Run](#contentrun).

### content.Run

Run is one element of a segment: either text or a tag, never both.

| Field | Type | Description |
|---|---|---|
| `tag` | [content.Tag](#contenttag) |  |
| `text` | string |  |

### content.Tag

Tag is an inline element. ID pairs open and close tags inside a segment. Type is a format-neutral hint ("bold", "link", "lb"). Desc describes the tag for people ("Colour #C00000, Bold"). Native holds the original markup so the file can be rebuilt exactly.

| Field | Type | Description |
|---|---|---|
| `desc` | string |  |
| `id` | string |  |
| `kind` | `open` \| `close` \| `standalone` |  |
| `native` | string |  |
| `type` | string |  |

### langdetect.Detection

Detection is a guess at a text's language.

| Field | Type | Description |
|---|---|---|
| `alternatives` | list of [langdetect.Guess](#langdetectguess) | Alternatives are the runners-up, best first. |
| `basis` | integer | Basis is the amount of text the guess rests on, in Unit. |
| `confidence` | number | Confidence is from 0 to 1; below Confident the person confirms. |
| `lang` | string | Lang is the best guess (a BCP 47 tag), empty when the text gives no usable evidence. |
| `reason` | [langdetect.Reason](#langdetectreason) |  |
| `unit` | string | Unit is "words", or "characters" for scripts written without spaces. |

### langdetect.Guess

Guess is another language the text may be in.

| Field | Type | Description |
|---|---|---|
| `confidence` | number |  |
| `lang` | string |  |

### langdetect.Reason

Reason says why there is a guess or none.

One of `empty`, `mixed`, `ok`, `short`.

### project.Analysis

Analysis is how the project's analysis counts.

| Field | Type | Description |
|---|---|---|
| `context` | string | Context is the context match rule: "previous" (Trados) or "both" (memoQ). |

### project.Comment

Comment is a remark on a segment by someone working on the project.

| Field | Type | Description |
|---|---|---|
| `at` | string | At is when it was written, in RFC 3339. |
| `by` | string |  |
| `id` | string |  |
| `text` | string |  |

### project.Edit

Edit is what saving a segment changes.

| Field | Type | Description |
|---|---|---|
| `ignored` | list of string |  |
| `origin` | [project.Origin](#projectorigin) |  |
| `status` | [project.Status](#projectstatus) |  |
| `target` | [content.Content](#contentcontent) |  |
| `warnings` | list of [project.Warning](#projectwarning) |  |

### project.File

File is a file of the project.

| Field | Type | Description |
|---|---|---|
| `name` | string | Name is the file name, with the folder it came from if any ("contracts/lease.docx"). Names are unique in a project, ignoring case. |
| `package` | string | Package is the file's path in the project's Trados package, for a file that came from one ("en-GB/Vertrag.docx.sdlxliff"). |
| `role` | [project.Role](#projectrole) |  |
| `targetLang` | string | TargetLang is set for a bilingual file (XLIFF, a file of a Trados package): it is translated into this language only. Empty for a monolingual file, which is translated into every target language. |

### project.FileQuality

FileQuality is the score of a file in a target language.

| Field | Type | Description |
|---|---|---|
| `categories` | map of integer | Categories counts the issues of each kind that has any. |
| `critical` | integer |  |
| `file` | string |  |
| `issues` | integer | Issues counts all issues; Minor, Major and Critical by severity. |
| `lang` | string |  |
| `major` | integer |  |
| `marked` | integer |  |
| `minor` | integer |  |
| `penalty` | integer | Penalty is the sum of the issues' points. |
| `perThousand` | number | PerThousand is the penalty for every thousand words, and Score is 100 less the penalty in percent of the words, never below 0; both to one decimal place. Without words both are 0 and 100. |
| `score` | number |  |
| `segments` | integer | Segments is how many segments there are, and Marked how many of them have an issue. |
| `words` | integer | Words is the number of source words of the segments. |

### project.FileState

FileState is a segment's state in its bilingual file.

| Field | Type | Description |
|---|---|---|
| `fixed` | boolean | Fixed: the program the file comes from keeps its own record of each segment (its ID, state and history), so the segment cannot be split or joined. |
| `match` | integer | Match is the match percentage the file records. |
| `state` | string | State is "new", "draft", "translated", "reviewed", "final" or "rejected". |
| `target` | string | Target is the translation in the file, as content JSON. |

### project.Issue

Issue is something a reviewer found wrong with a segment's translation.

| Field | Type | Description |
|---|---|---|
| `at` | string |  |
| `by` | string |  |
| `category` | string | Category is what kind of mistake it is, one of IssueCategories. |
| `id` | string |  |
| `note` | string | Note says what is wrong, if the reviewer wrote it down. |
| `severity` | string | Severity is how much it weighs, one of IssueSeverities. |

### project.JoinedUnit

JoinedUnit is one of the units of the file a stretch of several was joined from. Every part of such a stretch lists them.

| Field | Type | Description |
|---|---|---|
| `id` | string |  |
| `source` | [content.Content](#contentcontent) |  |

### project.Location

Location is where a termbase or memory is kept.

One of `local`, `server`.

### project.MemorySetting

MemorySetting is how a project uses one translation memory for one target language.

| Field | Type | Description |
|---|---|---|
| `concordance` | boolean | Concordance searches it with the concordance search. |
| `lookup` | boolean | Lookup offers its matches while translating, in the analysis and in pre-translation. |
| `memory` | string | Memory is the memory's name. |
| `penalty` | integer | Penalty lowers its match scores by this many points (0 to 100). |
| `update` | boolean | Update stores confirmed translations in it. |

### project.Origin

Origin is where a translation came from.

| Field | Type | Description |
|---|---|---|
| `context` | boolean |  |
| `from` | string | From is the segment a repetition was taken from. |
| `inserted` | string | Inserted is the translation as inserted, as content JSON, to see whether it was edited. |
| `kind` | string | Kind is "tm" (a memory match), "mt", "copy" (the source) or "repetition" (the translation of an equal segment). |
| `memory` | string |  |
| `score` | integer | Score is a memory match's score; Memory its memory; Context marks a context match. |

### project.Prepare

Prepare is what happens to the files when the project is created.

| Field | Type | Description |
|---|---|---|
| `confirmContext` | boolean | ConfirmContext confirms segments filled with a context match. |
| `confirmExact` | boolean | ConfirmExact confirms segments filled with a 100% match (context matches included). |
| `minScore` | integer | MinScore is the lowest match inserted (50 to 100). |
| `pretranslate` | boolean | Pretranslate fills empty segments with the best memory match. |

### project.Problem

Problem is one thing wrong with a setup.

| Field | Type | Description |
|---|---|---|
| `field` | string | Field names the setting: "name", "targetLangs", "files[1].name", "memories.en-GB[0].penalty", "terminology.writeTo". |
| `message` | string |  |

### project.QA

QA is which quality checks run while translating, as qa.Options switches them: a check runs if it is on by default and not disabled, or enabled.

| Field | Type | Description |
|---|---|---|
| `dictionaries` | map of string | Dictionaries names the spelling dictionary for each target language; a language without one is not spell-checked. |
| `disabled` | list of [qa.Kind](#qakind) |  |
| `enabled` | list of [qa.Kind](#qakind) |  |
| `grammar` | string | Grammar names the grammar service that reads the translations of this project; empty for none. A translation goes to the service while it is worked on. |
| `rules` | list of [qa.UserRule](#qauserrule) | Rules are checks of the project's own. |
| `uneditedFuzzy` | boolean | UneditedFuzzy warns when a fuzzy match is confirmed unchanged. |
| `words` | list of string | Words are always spelled right in this project (names, terms). |

### project.Quality

Quality is the score of segments from the issues marked on them.

| Field | Type | Description |
|---|---|---|
| `categories` | map of integer | Categories counts the issues of each kind that has any. |
| `critical` | integer |  |
| `issues` | integer | Issues counts all issues; Minor, Major and Critical by severity. |
| `major` | integer |  |
| `marked` | integer |  |
| `minor` | integer |  |
| `penalty` | integer | Penalty is the sum of the issues' points. |
| `perThousand` | number | PerThousand is the penalty for every thousand words, and Score is 100 less the penalty in percent of the words, never below 0; both to one decimal place. Without words both are 0 and 100. |
| `score` | number |  |
| `segments` | integer | Segments is how many segments there are, and Marked how many of them have an issue. |
| `words` | integer | Words is the number of source words of the segments. |

### project.Review

Review is the scored review of files: each file's score and the score of all together.

| Field | Type | Description |
|---|---|---|
| `files` | list of [project.FileQuality](#projectfilequality) |  |
| `total` | [project.Quality](#projectquality) |  |

### project.Role

Role says what a file is for.

One of `package`, `reference`, `translatable`.

### project.Segment

Segment is a segment of a file in one target language.

| Field | Type | Description |
|---|---|---|
| `block` | string | Block is the paragraph of the original document the segment is in. |
| `comments` | list of [project.Comment](#projectcomment) | Comments are what the people working on the project wrote about the segment, oldest first. They do not go into the document. |
| `file` | [project.FileState](#projectfilestate) | File is the segment's state and translation in a bilingual file, so writing the file back keeps a reviewer's approval of a translation that did not change. |
| `id` | string |  |
| `ignored` | list of string | Ignored are the warnings marked as false alarms, by kind and text. |
| `issues` | list of [project.Issue](#projectissue) | Issues are what a reviewer marked as wrong with the translation; the file's score comes from them. |
| `joined` | list of [project.JoinedUnit](#projectjoinedunit) |  |
| `locked` | boolean |  |
| `maxLength` | integer | MaxLength is the longest the translation may be, in characters. |
| `notes` | list of string | Notes are a bilingual file's comments on the segment. |
| `origin` | [project.Origin](#projectorigin) | Origin is where the translation came from when it was inserted. |
| `source` | [content.Content](#contentcontent) |  |
| `split` | [project.SplitPart](#projectsplitpart) | Split says the segment is a part of a unit of the file that was split, and Joined lists the units of the file a joined segment was made of; the segment's ID is then the first unit's. |
| `status` | [project.Status](#projectstatus) |  |
| `target` | [content.Content](#contentcontent) |  |
| `warnings` | list of [project.Warning](#projectwarning) | Warnings are the checks' results saved when it was confirmed. |

### project.SegmentationRule

SegmentationRule is a rule of the project's own for where sentences end: at a place where the text before it ends with Before and the text after it begins with After (regular expressions, either may be empty), a segment ends (Break) or does not. The project's rules come before the ones Interlinea has for the language, and the first rule that matches a place decides it.

| Field | Type | Description |
|---|---|---|
| `after` | string |  |
| `before` | string |  |
| `break` | boolean |  |

### project.Settings

Settings is a project as it is set up.

| Field | Type | Description |
|---|---|---|
| `analysis` | [project.Analysis](#projectanalysis) |  |
| `client` | string |  |
| `description` | string |  |
| `due` | string | Due is the deadline, RFC 3339 ("2026-12-04T16:00:00Z"). |
| `files` | list of [project.File](#projectfile) |  |
| `memories` | map of list of [project.MemorySetting](#projectmemorysetting) | Memories are the memories per target language, in priority order (the first wins a tie between equal matches). |
| `name` | string |  |
| `prepare` | [project.Prepare](#projectprepare) |  |
| `projectMemory` | boolean | ProjectMemory gives the project its own memory per target language, which collects its confirmed translations. |
| `qa` | [project.QA](#projectqa) |  |
| `segmentation` | list of [project.SegmentationRule](#projectsegmentationrule) | Segmentation are the project's own rules for where sentences end, applied to the files added to it. |
| `sourceLang` | string |  |
| `targetLangs` | list of string |  |
| `terminology` | [project.Terminology](#projectterminology) |  |

### project.SplitPart

SplitPart says that a segment is a part of a stretch that was split.

| Field | Type | Description |
|---|---|---|
| `space` | string | Space is the whitespace that stood before the part where the stretch was cut; the first part has none. |
| `unit` | string | Unit is the ID of the first unit of the stretch the part belongs to. |

### project.Status

Status is a segment's translation status.

One of `confirmed`, `draft`, `new`.

### project.Template

Template is what a new project can start from: everything but the project's name, files and dates.

| Field | Type | Description |
|---|---|---|
| `analysis` | [project.Analysis](#projectanalysis) |  |
| `client` | string |  |
| `description` | string |  |
| `memories` | map of list of [project.MemorySetting](#projectmemorysetting) |  |
| `name` | string |  |
| `prepare` | [project.Prepare](#projectprepare) |  |
| `projectMemory` | boolean |  |
| `qa` | [project.QA](#projectqa) |  |
| `segmentation` | list of [project.SegmentationRule](#projectsegmentationrule) |  |
| `sourceLang` | string |  |
| `targetLangs` | list of string |  |
| `terminology` | [project.Terminology](#projectterminology) |  |

### project.TermMode

TermMode says where terms found while translating are stored.

One of `direct`, `layered`, `own`.

### project.Terminology

Terminology is the project's termbase setup.

| Field | Type | Description |
|---|---|---|
| `addLanguages` | list of string | AddLanguages lists masters that gain the project languages they lack. The masters are changed only when the user chose this. |
| `autoUpdate` | boolean | AutoUpdate takes in the masters' changes each time the project opens (Layered). Otherwise they wait until the user updates. |
| `location` | [project.Location](#projectlocation) | Location is where the project termbase is kept (Layered, Own). |
| `masters` | list of string | Masters are the master termbases used, by ID. |
| `mode` | [project.TermMode](#projecttermmode) |  |
| `termbase` | string | Termbase is the project termbase's ID once it exists (Layered, Own); empty until it is made, and in Direct mode. |
| `writeTo` | string | WriteTo is the master new terms go into in Direct mode. |

### project.Warning

Warning is a check's result on a segment.

| Field | Type | Description |
|---|---|---|
| `kind` | string |  |
| `text` | string |  |

### projectdb.Assignment

Assignment gives a person part of a project: a target language of a file, all of a file's languages, or all files, and within one file a range of segments.

| Field | Type | Description |
|---|---|---|
| `changedAt` | string (date-time) |  |
| `changedBy` | string |  |
| `done` | boolean | Done is set when the person reports the work finished. |
| `due` | string | Due is the deadline, RFC 3339; empty for the project's. |
| `file` | string | File is the file's name; empty for every file to translate. |
| `fromSegment` | string | FromSegment and ToSegment bound a range of segments by their IDs, both included; empty for the whole file. Only with a File. |
| `id` | integer |  |
| `projectId` | integer |  |
| `targetLang` | string | TargetLang is the language; empty for every target language. |
| `task` | [projectdb.Task](#projectdbtask) |  |
| `toSegment` | string |  |
| `userId` | integer | UserID and UserName identify the person (an authdb account). |
| `userName` | string |  |
| `version` | integer |  |

### projectdb.Change

Change is one entry of the change log. For template changes Project is the template's name and ProjectID 0.

| Field | Type | Description |
|---|---|---|
| `at` | string (date-time) |  |
| `by` | string |  |
| `file` | string |  |
| `kind` | string |  |
| `project` | string |  |
| `projectId` | integer |  |
| `segment` | string |  |
| `seq` | integer |  |
| `targetLang` | string |  |
| `version` | integer |  |

### projectdb.Dictionary

Dictionary is a spelling dictionary (Hunspell .aff and .dic files) the projects' quality checks spell with.

| Field | Type | Description |
|---|---|---|
| `createdAt` | string (date-time) |  |
| `createdBy` | string |  |
| `lang` | string | Lang is the language it spells (a BCP 47 tag). |
| `locales` | list of string | Locales are the languages its archive declared for it. |
| `name` | string | Name identifies it ("de_DE_frami"). |
| `words` | integer | Words counts its words. |

### projectdb.Presence

Presence marks who has a segment open.

| Field | Type | Description |
|---|---|---|
| `expires` | string (date-time) |  |
| `file` | string |  |
| `segment` | string |  |
| `since` | string (date-time) |  |
| `targetLang` | string |  |
| `userId` | integer |  |
| `userName` | string |  |

### projectdb.StoredSegment

StoredSegment is a segment with its version.

| Field | Type | Description |
|---|---|---|
| `block` | string | Block is the paragraph of the original document the segment is in. |
| `changedAt` | string (date-time) |  |
| `changedBy` | string |  |
| `comments` | list of [project.Comment](#projectcomment) | Comments are what the people working on the project wrote about the segment, oldest first. They do not go into the document. |
| `file` | [project.FileState](#projectfilestate) | File is the segment's state and translation in a bilingual file, so writing the file back keeps a reviewer's approval of a translation that did not change. |
| `id` | string |  |
| `ignored` | list of string | Ignored are the warnings marked as false alarms, by kind and text. |
| `issues` | list of [project.Issue](#projectissue) | Issues are what a reviewer marked as wrong with the translation; the file's score comes from them. |
| `joined` | list of [project.JoinedUnit](#projectjoinedunit) |  |
| `locked` | boolean |  |
| `maxLength` | integer | MaxLength is the longest the translation may be, in characters. |
| `notes` | list of string | Notes are a bilingual file's comments on the segment. |
| `origin` | [project.Origin](#projectorigin) | Origin is where the translation came from when it was inserted. |
| `source` | [content.Content](#contentcontent) |  |
| `split` | [project.SplitPart](#projectsplitpart) | Split says the segment is a part of a unit of the file that was split, and Joined lists the units of the file a joined segment was made of; the segment's ID is then the first unit's. |
| `status` | [project.Status](#projectstatus) |  |
| `target` | [content.Content](#contentcontent) |  |
| `version` | integer |  |
| `warnings` | list of [project.Warning](#projectwarning) | Warnings are the checks' results saved when it was confirmed. |

### projectdb.StoredTemplate

StoredTemplate is a template with its version.

| Field | Type | Description |
|---|---|---|
| `analysis` | [project.Analysis](#projectanalysis) |  |
| `changedAt` | string (date-time) |  |
| `changedBy` | string |  |
| `client` | string |  |
| `description` | string |  |
| `memories` | map of list of [project.MemorySetting](#projectmemorysetting) |  |
| `name` | string |  |
| `prepare` | [project.Prepare](#projectprepare) |  |
| `projectMemory` | boolean |  |
| `qa` | [project.QA](#projectqa) |  |
| `segmentation` | list of [project.SegmentationRule](#projectsegmentationrule) |  |
| `sourceLang` | string |  |
| `targetLangs` | list of string |  |
| `terminology` | [project.Terminology](#projectterminology) |  |
| `version` | integer |  |

### projectdb.Task

Task is what an assignment asks of someone.

One of `review`, `translate`.

### qa.Issue

Issue is a problem found in a segment.

| Field | Type | Description |
|---|---|---|
| `found` | string | Found is the text the issue is about, as it appears in the translation; Suggestions are replacements (spelling). |
| `kind` | [qa.Kind](#qakind) |  |
| `message` | string |  |
| `rule` | string | Rule names the user rule that found the issue. |
| `segment` | string |  |
| `severity` | [qa.Severity](#qaseverity) |  |
| `suggestions` | list of string |  |

### qa.Kind

Kind names a check.

One of `brackets`, `capitalization`, `double-space`, `edge-space`, `empty`, `end-punctuation`, `grammar`, `inconsistent`, `inconsistent-source`, `length`, `length-ratio`, `numbers-extra`, `numbers-missing`, `repeated-word`, `rule`, `space-before-punctuation`, `spelling`, `tag-order`, `tags-extra`, `tags-missing`, `term-forbidden`, `term-missing`, `untranslated`.

### qa.Severity

Severity says how serious an issue is.

One of `error`, `warning`.

### qa.UserRule

UserRule is a check of the user's own: a regular expression the translation must or must not match, optionally only where the source matches another one. With Mode "count", the translation must match Target as often as the source matches Source.

| Field | Type | Description |
|---|---|---|
| `message` | string |  |
| `mode` | string | "must", "must-not" or "count" |
| `name` | string |  |
| `severity` | [qa.Severity](#qaseverity) |  |
| `source` | string |  |
| `target` | string |  |

### term.Change

Change is a difference between two versions of an entry, for showing a reviewer what a project changed.

| Field | Type | Description |
|---|---|---|
| `new` | string |  |
| `old` | string |  |
| `path` | string |  |

### term.Column

Column maps one spreadsheet column.

| Field | Type | Description |
|---|---|---|
| `field` | string |  |
| `header` | string |  |
| `index` | integer |  |
| `lang` | string |  |
| `level` | [term.Level](#termlevel) |  |
| `role` | [term.Role](#termrole) |  |

### term.Conflict

Conflict is a place where both sides changed an entry differently since their common version.

| Field | Type | Description |
|---|---|---|
| `base` | string | Base, Ours and Theirs are the values in the common version, in our version (the project) and in theirs (the master); "" where absent. |
| `ours` | string |  |
| `path` | string | Path names the place in words: "entry field Subject", "de-DE language field Note", "de-DE term Kaution", "de-DE term Kaution status", "de-DE term Kaution field Source". |
| `theirs` | string |  |

### term.Definition

Definition is a termbase's structure: languages and fields.

| Field | Type | Description |
|---|---|---|
| `fields` | list of [term.Field](#termfield) |  |
| `languages` | list of string |  |
| `name` | string |  |

### term.Entry

Entry is a concept with its terms in several languages.

| Field | Type | Description |
|---|---|---|
| `changedAt` | string (date-time) |  |
| `changedBy` | string |  |
| `createdAt` | string (date-time) |  |
| `createdBy` | string |  |
| `fields` | list of [term.Value](#termvalue) |  |
| `id` | string |  |
| `languages` | list of [term.Language](#termlanguage) |  |

### term.Field

Field describes a field of the termbase definition.

| Field | Type | Description |
|---|---|---|
| `kind` | [term.Kind](#termkind) |  |
| `level` | [term.Level](#termlevel) |  |
| `name` | string |  |
| `values` | list of string | picklist values |

### term.Format

Format is a termbase exchange format.

One of `csv`, `multiterm`, `sdltb`, `tbx`, `tbx2`, `tsv`, `xdt`, `xlsx`.

### term.Kind

Kind is the type of a field's values.

One of `boolean`, `date`, `number`, `picklist`, `text`.

### term.Language

Language is a language section of an entry.

| Field | Type | Description |
|---|---|---|
| `fields` | list of [term.Value](#termvalue) |  |
| `lang` | string |  |
| `terms` | list of [term.Term](#termterm) |  |

### term.Level

Level says where a field lives.

One of `entry`, `language`, `term`.

### term.Mapping

Mapping says how to read a spreadsheet, as memoQ's and MultiTerm Convert's column mapping do.

| Field | Type | Description |
|---|---|---|
| `columns` | list of [term.Column](#termcolumn) |  |
| `header` | boolean | Header is true when the first row holds column names. |
| `separator` | string | Separator splits several terms (synonyms) in one cell; default ";". |

### term.Report

Report tells what an import read and what it had to leave out.

| Field | Type | Description |
|---|---|---|
| `entries` | integer |  |
| `skipped` | integer |  |
| `terms` | integer |  |
| `warnings` | list of string |  |

### term.Role

Role is what a spreadsheet column holds.

One of `field`, `id`, `ignore`, `status`, `term`.

### term.Term

Term is a term with its status and fields.

| Field | Type | Description |
|---|---|---|
| `fields` | list of [term.Value](#termvalue) |  |
| `status` | string |  |
| `text` | string |  |

### term.Value

Value is one field value. Order is kept as in the source file.

| Field | Type | Description |
|---|---|---|
| `field` | string |  |
| `value` | string |  |

### termdb.Assignment

Assignment is a reviewer's part of a request.

| Field | Type | Description |
|---|---|---|
| `reviewer` | string |  |
| `scope` | [termdb.Filter](#termdbfilter) |  |

### termdb.BaseRef

BaseRef is the master entry and version a project entry started from.

| Field | Type | Description |
|---|---|---|
| `entryId` | string |  |
| `termbase` | string |  |
| `version` | integer |  |

### termdb.Change

Change is one entry of the change log: every created, changed and deleted entry and every change to a termbase itself, in the order they happened. Seq increases with every change across the whole database, so a client that has seen everything up to Seq asks for what came after it and is up to date.

| Field | Type | Description |
|---|---|---|
| `at` | string (date-time) |  |
| `by` | string |  |
| `entryId` | string |  |
| `kind` | string |  |
| `seq` | integer |  |
| `summary` | string |  |
| `termbase` | string |  |
| `version` | integer |  |

### termdb.DecideResult

DecideResult says what a decision did.

| Field | Type | Description |
|---|---|---|
| `decided` | integer |  |
| `needResolution` | list of integer | NeedResolution are the items left waiting because the master changed the same places; decide them one by one with an edited entry. |

### termdb.Decision

Decision is what a reviewer decides.

| Field | Type | Description |
|---|---|---|
| `comment` | string |  |
| `edited` | [term.Entry](#termentry) | Edited is the entry to merge instead of the project's (with StateEdited), for example the resolution of a conflict. Only for a single item. |
| `state` | string | State is StateApproved, StateEdited, StateChanges, StateRejected or StatePostponed. |

### termdb.Deletion

Deletion records a deleted entry.

| Field | Type | Description |
|---|---|---|
| `at` | string (date-time) |  |
| `by` | string |  |
| `entry` | [term.Entry](#termentry) |  |
| `entryId` | string |  |
| `summary` | string |  |

### termdb.Filter

Filter selects entries of a merge request. Empty fields do not restrict; a filter with several fields selects what matches all of them.

| Field | Type | Description |
|---|---|---|
| `authors` | list of string |  |
| `conflicts` | boolean | Conflicts: only entries with (true) or without (false) conflicts. |
| `fields` | list of [term.Value](#termvalue) | Fields: the entry has these field values (at any level; values compared without case). |
| `ids` | list of integer | IDs selects items by ID. |
| `kinds` | list of string |  |
| `langs` | list of string | Langs: the entry has terms in one of these languages (or their variants). |
| `since` | string (date-time) | Since and Until bound the time of the project's change. |
| `states` | list of string |  |
| `statuses` | list of string | Statuses: the entry has a term with one of these statuses. |
| `text` | string | Text: a term contains this text (case aside). |
| `until` | string (date-time) |  |

### termdb.Item

Item is one entry of a merge request with what the reviewer needs to decide it.

| Field | Type | Description |
|---|---|---|
| `author` | string |  |
| `base` | [termdb.BaseRef](#termdbbaseref) |  |
| `changedAt` | string (date-time) |  |
| `changes` | list of [term.Change](#termchange) | Changes are what the project changed since its copy (all of the entry for a new one); Conflicts where the master changed the same places since then. Merged and MergedOurs are the two ways out of a conflict: both have every change either side made alone, and at the conflicting places Merged keeps the master's value and MergedOurs takes the project's. MergedOurs is set only where they differ. |
| `comment` | string |  |
| `conflicts` | list of [term.Conflict](#termconflict) |  |
| `decidedAt` | string (date-time) |  |
| `duplicates` | list of string |  |
| `entry` | [term.Entry](#termentry) | Entry is the project's version (for a deletion, the master entry the project removed); Master the master's current version, if any. |
| `entryId` | string |  |
| `forbidden` | boolean | Forbidden is set if the entry has a forbidden term; Duplicates lists master entries that share a term with it (new entries). |
| `frequency` | integer | Frequency is how often the entry's terms occur in the project's documents, as the caller counted (ItemQuery.Frequency). |
| `id` | integer |  |
| `kind` | string |  |
| `master` | [termdb.Stored](#termdbstored) |  |
| `merged` | [term.Entry](#termentry) |  |
| `mergedOurs` | [term.Entry](#termentry) |  |
| `projectVersion` | integer |  |
| `result` | [termdb.BaseRef](#termdbbaseref) | Result is the master entry the approval wrote. |
| `reviewer` | string |  |
| `reviewers` | list of string | Reviewers are those whose assigned scope includes the entry. |
| `rule` | string | Rule names the review rule that decided the entry, if one did. |
| `state` | string |  |

### termdb.Layered

Layered is an entry as a project shows it.

| Field | Type | Description |
|---|---|---|
| `base` | [termdb.BaseRef](#termdbbaseref) | Base is set for a project entry changed from a master entry. |
| `changedAt` | string (date-time) |  |
| `changedBy` | string |  |
| `createdAt` | string (date-time) |  |
| `createdBy` | string |  |
| `fields` | list of [term.Value](#termvalue) |  |
| `id` | string |  |
| `languages` | list of [term.Language](#termlanguage) |  |
| `termbase` | string | Termbase is where the entry lives: the project or a master. |
| `version` | integer |  |

### termdb.MergeRequest

MergeRequest is a request to merge a project's changes into a master.

| Field | Type | Description |
|---|---|---|
| `closedAt` | string (date-time) |  |
| `createdAt` | string (date-time) |  |
| `createdBy` | string |  |
| `deadline` | string (date-time) |  |
| `id` | integer |  |
| `project` | string |  |
| `state` | string | open or closed |
| `target` | string |  |
| `title` | string |  |

### termdb.Progress

Progress counts the request's items per state, in total and per assigned reviewer.

| Field | Type | Description |
|---|---|---|
| `byReviewer` | map of map of integer |  |
| `conflicts` | integer |  |
| `states` | map of integer |  |
| `total` | integer |  |

### termdb.Removal

Removal is a master entry a project has removed.

| Field | Type | Description |
|---|---|---|
| `at` | string (date-time) |  |
| `by` | string |  |
| `entryId` | string |  |
| `termbase` | string |  |
| `version` | integer |  |

### termdb.ReviewRules

ReviewRules are a master's rules for entries that need no review. The zero value merges entries identical to the master's and nothing else.

| Field | Type | Description |
|---|---|---|
| `keepIdentical` | boolean | KeepIdentical turns off merging, without review, of entries that are already in the master as they are. |
| `minorFields` | list of string | MinorFields are fields whose changes need no approval (notes, sources): a change to these fields only is merged at once. |
| `trusted` | list of [termdb.TrustedRule](#termdbtrustedrule) | Trusted approves the entries of these authors in their scope without review. |

### termdb.Stored

Stored is an entry with the version it has in the database.

| Field | Type | Description |
|---|---|---|
| `changedAt` | string (date-time) |  |
| `changedBy` | string |  |
| `createdAt` | string (date-time) |  |
| `createdBy` | string |  |
| `fields` | list of [term.Value](#termvalue) |  |
| `id` | string |  |
| `languages` | list of [term.Language](#termlanguage) |  |
| `version` | integer |  |

### termdb.TrustedRule

TrustedRule approves an author's entries within a scope.

| Field | Type | Description |
|---|---|---|
| `author` | string |  |
| `scope` | [termdb.Filter](#termdbfilter) |  |

### termdb.Version

Version is one state of an entry in its history.

| Field | Type | Description |
|---|---|---|
| `at` | string (date-time) |  |
| `by` | string |  |
| `entry` | [term.Entry](#termentry) |  |
| `summary` | string |  |
| `version` | integer |  |

### tm.Info

Info describes a memory.

| Field | Type | Description |
|---|---|---|
| `Client` | string | Client is who the memory is kept for, if anyone. |
| `Name` | string |  |
| `SourceLang` | string |  |
| `TargetLang` | string |  |

### tmdb.Change

Change is one entry of the change log. Seq increases with every change across the database; a client that has seen everything up to Seq asks for what came after it.

| Field | Type | Description |
|---|---|---|
| `at` | string (date-time) |  |
| `by` | string |  |
| `kind` | string |  |
| `memory` | string |  |
| `seq` | integer |  |
| `unitId` | integer |  |
| `version` | integer |  |

