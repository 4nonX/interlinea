# Surface brief: prototype (project creation, termbase setup, terminology, merge review)

Scope: `prototype/` (clickable React prototype, mock data). Mode: Operate.
Audience: professional translators (first user: freelance sworn translator,
DE/EN legal). Job: start a project from a document with the right termbases
in under a minute; manage terminology; merge project terminology into
masters with a review.
Constraints: plain/git wording switch; local and server termbases side by
side; WCAG 2.2 AA; `lang` on all term text; keyboard complete.
Build path: code-led (Operate). Launcher not run (no concept-seed roll).

## Direction contract

THESIS: A translator's tool that sets itself up: the document decides the
languages, the client decides the termbases, and terminology changes behave
like a branch that is reviewed back into the master. Refuses the
database-first wizard (fields, levels, definitions before any term).

OWN-WORLD: Category standard, played straight at the craft level of memoQ 11
and Phrase TMS: light, cool neutral shell, white work surfaces, one blue
accent for primary actions and selection, semantic status colours only where
they carry meaning, platform UI font, tabular numerals, dense but calm.

STORY: The visitor drops a document, sees its language detected with the
evidence, picks targets, chooses project termbase plus masters (local or
server), creates the project; later reviews project changes against the
master, resolves a conflict, merges, and sees the history.

FIRST VIEWPORT: Left rail navigation (Projects, Terminology, Settings).
Projects page: drop zone for a document as the primary action at the top of
the work area, recent projects below with their termbase setup and pending
merge counts.

FORM: Category standard (canon, chosen by the user over three grounded
directions). seed: none (unavailable)

FINISH: unreviewed and undocumented is unfinished; this build ends with the
finish review, the verdict, DESIGN.md, and every shipping raster carrying its
provenance
