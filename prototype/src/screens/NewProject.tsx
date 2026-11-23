import { useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  CheckCircle2,
  FileText,
  Info,
  Plus,
  X,
} from "lucide-react";
import {
  LANGUAGES,
  SAMPLE_DOCS,
  lang,
  primary,
  type LangCode,
  type Location,
  type SetupMode,
} from "../data";
import {
  CONFIDENT,
  detectLanguage,
  readDocument,
  readPackageFile,
  type Detection,
} from "../detect";
import { extractDocx, XLIFF_FORMAT_NAME, type Extracted } from "../core";
import { analyze } from "../editor/analysis";
import { clientMemories } from "../editor/memory";
import { core, loadCore } from "../core";
import { segmentText } from "../editor/model";
import { AnalysisTable } from "../components/AnalysisTable";
import { takePendingDoc, type PendingDoc } from "../draft";
import type { Segment } from "../editor/model";
import { useStore, useWords, type NewProjectInput } from "../store";
import { fmtNum, plural } from "../format";
import { LangChip, LocationBadge, PageHeader } from "../components/ui";
import { go } from "../App";

const STEPS = ["Languages", "Terminology", "Review"] as const;

function nameFromFile(file: string): string {
  return file
    .replace(/\.[^.]+$/, "")
    .replace(/[_]+/g, " ")
    .trim();
}

export function NewProject({ sample }: { sample?: number }) {
  const { state, dispatch, notify } = useStore();
  const w = useWords();
  const [doc, setDoc] = useState<PendingDoc | null>(() => {
    const pending = takePendingDoc();
    if (pending) return pending;
    // Deep link to a sample document: #/new/sample/<index>
    const s = sample !== undefined ? SAMPLE_DOCS[sample] : undefined;
    return s
      ? { file: s.file, words: s.words, text: s.text || null, client: s.client }
      : null;
  });
  const [step, setStep] = useState(0);
  const stepHeading = useRef<HTMLHeadingElement>(null);

  // Most frequent source and target languages of earlier projects: the fallback
  // when a document gives no usable text.
  const recentSource = useMemo(
    () => mostCommon(state.projects.map((p) => p.sourceLang)) ?? "de-DE",
    [state.projects],
  );
  const recentTargets = useMemo(
    () => [...new Set(state.projects.flatMap((p) => p.targetLangs))],
    [state.projects],
  );
  const clients = useMemo(
    () => [
      ...new Set(
        state.termbases.map((t) => t.client).filter(Boolean) as string[],
      ),
    ],
    [state.termbases],
  );

  const detection: Detection | null = useMemo(
    () => (doc ? detectLanguage(doc.text ?? "") : null),
    [doc],
  );
  const [source, setSource] = useState<LangCode>("");
  const [sourceTouched, setSourceTouched] = useState(false);
  const [targets, setTargets] = useState<LangCode[]>([]);
  const [name, setName] = useState("");
  const [client, setClient] = useState("");

  useEffect(() => {
    if (!doc) return;
    setName(nameFromFile(doc.file));
    setClient(doc.client ?? "");
    if (doc.bilingual) {
      // A bilingual file names its languages; they cannot change.
      setSource(knownLang(doc.bilingual.sourceLang));
      setSourceTouched(true);
      setTargets(
        doc.bilingual.targetLang ? [knownLang(doc.bilingual.targetLang)] : [],
      );
      return;
    }
    const detected = detection?.code ?? null;
    const src = detected ?? recentSource;
    setSource(src);
    setSourceTouched(false);
    const suggested = recentTargets.filter((t) => primary(t) !== primary(src));
    setTargets(
      suggested.length
        ? [suggested[0]!]
        : [primary(src) === "en" ? "de-DE" : "en-GB"],
    );
  }, [doc]);

  // Terminology choices.
  const masters = state.termbases.filter((t) => t.kind === "master");
  const [selectedMasters, setSelectedMasters] = useState<string[]>([]);
  const [mode, setMode] = useState<SetupMode>("layered");
  const [directTarget, setDirectTarget] = useState<string>("");
  const [location, setLocation] = useState<Location>("local");
  const [autoUpdate, setAutoUpdate] = useState(true);
  const [mastersTouched, setMastersTouched] = useState(false);

  // Suggest masters for the client and the document's source language until
  // the user changes the selection.
  useEffect(() => {
    if (mastersTouched) return;
    const byClient = masters
      .filter((m) => client && m.client === client)
      .map((m) => m.id);
    setSelectedMasters(byClient);
  }, [client, mastersTouched]);

  useEffect(() => {
    if (selectedMasters.length === 0 && mode !== "own") setMode("own");
    if (selectedMasters.length > 0 && mode === "own" && !mastersTouched)
      setMode("layered");
    if (!selectedMasters.includes(directTarget))
      setDirectTarget(selectedMasters[0] ?? "");
    const serverMaster = state.termbases.find(
      (t) => selectedMasters.includes(t.id) && t.location === "server",
    );
    if (!mastersTouched) setLocation(serverMaster ? "server" : "local");
  }, [selectedMasters]);

  useEffect(() => {
    stepHeading.current?.focus();
  }, [step]);

  const serverAvailable = state.termbases.some((t) => t.location === "server");
  const projectLangs = [source, ...targets];
  const targetError =
    targets.length === 0
      ? "Choose at least one target language."
      : targets.includes(source)
        ? "A target language can't be the same as the source language."
        : null;
  const nameError = name.trim() ? null : "Give the project a name.";

  const [creating, setCreating] = useState(false);
  // Word documents are extracted again with the chosen source language when
  // the review step opens; the analysis and the new project both use it.
  const [prepared, setPrepared] = useState<{
    lang: LangCode;
    ex: Extracted;
  } | null>(null);
  useEffect(() => {
    if (
      step !== 2 ||
      !doc?.bytes ||
      doc.bilingual ||
      !source ||
      prepared?.lang === source
    )
      return;
    let live = true;
    extractDocx(doc.bytes, source).then(
      (ex) => live && setPrepared({ lang: source, ex }),
      () => undefined,
    );
    return () => {
      live = false;
    };
  }, [step, doc, source, prepared?.lang]);
  const analysisSegments: Segment[] | null = useMemo(() => {
    if (doc?.bilingual) return doc.bilingual.segments;
    if (doc?.bytes)
      return prepared?.lang === source ? prepared.ex.segments : null;
    return doc?.text
      ? segmentText(doc.text).map((t, i) => ({
          id: `a${i}`,
          source: [t],
          target: [],
          status: "new" as const,
        }))
      : [];
  }, [doc, prepared, source]);
  const [coreReady, setCoreReady] = useState(() => core() !== null);
  useEffect(() => {
    if (step === 2 && !coreReady)
      loadCore().then(
        () => setCoreReady(true),
        () => undefined,
      );
  }, [step, coreReady]);
  const analysisMemory = useMemo(() => {
    const c = coreReady ? core() : null;
    return c
      ? clientMemories(c, {
          client: client.trim(),
          sourceLang: source,
          targetLangs: targets,
        })
      : [];
  }, [coreReady, client, source, targets]);
  const analysis = useMemo(() => {
    const c = coreReady ? core() : null;
    return analysisSegments && c
      ? analyze(c, analysisMemory, analysisSegments, state.contextRule)
      : null;
  }, [coreReady, analysisSegments, analysisMemory, state.contextRule]);

  async function create() {
    const id = `p-${Date.now().toString(36)}`;
    const usedMasters =
      mode === "direct"
        ? [directTarget]
        : mode === "layered"
          ? selectedMasters
          : [];
    let extracted: NewProjectInput["extracted"];
    if (doc!.bilingual && doc!.bytes) {
      extracted = {
        segments: doc!.bilingual.segments,
        blocks: doc!.bilingual.blocks,
        bytes: doc!.bytes,
        format: "xliff",
        sdlPackage: doc!.sdlPackage && {
          bytes: doc!.sdlPackage.bytes,
          name: doc!.sdlPackage.name,
          path: doc!.sdlPackage.path,
        },
      };
    } else if (doc!.bytes) {
      setCreating(true);
      try {
        // Sentence breaks follow the confirmed source language.
        const ex =
          prepared?.lang === source
            ? prepared.ex
            : await extractDocx(doc!.bytes, source);
        extracted = { ...ex, bytes: doc!.bytes, format: "docx" };
      } catch (e) {
        notify(
          `Could not read ${doc!.file}: ${e instanceof Error ? e.message : String(e)}`,
        );
        setCreating(false);
        return;
      }
    }
    dispatch({
      type: "createProject",
      id,
      now: new Date().toISOString(),
      input: {
        name: name.trim(),
        file: doc!.file,
        words: doc!.words,
        sourceLang: source,
        targetLangs: targets,
        client: client.trim() || undefined,
        mode,
        masters: usedMasters,
        location,
        text: doc!.text,
        extracted,
      },
    });
    notify(`Created project “${name.trim()}”`);
    go({ page: "editor", projectId: id });
  }

  if (!doc) return <ChooseDocument onChosen={setDoc} />;

  return (
    <div className="page page-narrow">
      <PageHeader
        title="New project"
        description={
          <>
            From <strong>{doc.file}</strong>
          </>
        }
      >
        <a className="btn btn-quiet" href="#/projects">
          Cancel
        </a>
      </PageHeader>

      <ol className="stepper" aria-label="Steps">
        {STEPS.map((s, i) => (
          <li
            key={s}
            aria-current={i === step ? "step" : undefined}
            className={i < step ? "is-done" : undefined}
          >
            <span className="stepper-num" aria-hidden="true">
              {i < step ? <CheckCircle2 size={16} /> : i + 1}
            </span>
            {i < step ? (
              <button
                type="button"
                className="link-button"
                onClick={() => setStep(i)}
              >
                {s}
              </button>
            ) : (
              <span>{s}</span>
            )}
          </li>
        ))}
      </ol>

      {step === 0 && (
        <section aria-labelledby="step-title" className="panel">
          <h2
            id="step-title"
            ref={stepHeading}
            tabIndex={-1}
            className="panel-title"
          >
            Languages
          </h2>

          <div className="file-row">
            <FileText size={20} aria-hidden="true" />
            <div>
              <div className="cell-title">{doc.file}</div>
              <div className="cell-sub">
                {doc.words
                  ? plural(doc.words, "word", "words")
                  : "Word count after import"}
                {doc.bilingual
                  ? `, ${doc.sdlPackage ? `from the Trados package ${doc.sdlPackage.name}` : (XLIFF_FORMAT_NAME[doc.bilingual.format] ?? "XLIFF")}, ${bilingualSummary(doc.bilingual.segments)}`
                  : doc.bytes && ", formatting kept by the Word filter"}
              </div>
            </div>
            <button
              type="button"
              className="btn btn-quiet btn-small file-row-action"
              onClick={() => setDoc(null)}
            >
              Choose another file
            </button>
          </div>
          {doc.warnings?.map((w) => (
            <p key={w} className="detect detect-warn">
              <AlertTriangle size={16} aria-hidden="true" />
              <span>{w}</span>
            </p>
          ))}
          {doc.error && (
            <p className="detect detect-warn" role="alert">
              <AlertTriangle size={16} aria-hidden="true" />
              <span>
                {doc.file} could not be read: {doc.error}
              </span>
            </p>
          )}

          {doc.sdlPackage && doc.sdlPackage.files.length > 1 && (
            <div className="field">
              <label htmlFor="pkg-file">File of the package to translate</label>
              <select
                id="pkg-file"
                value={doc.sdlPackage.path}
                onChange={async (e) => {
                  const p = doc.sdlPackage!;
                  setDoc(
                    await readPackageFile(
                      p.name,
                      p.bytes,
                      e.target.value,
                      p.files,
                    ),
                  );
                }}
              >
                {doc.sdlPackage.files.map((f) => (
                  <option key={f} value={f}>
                    {f}
                  </option>
                ))}
              </select>
              <p className="help">
                A Trados package can hold several files; each becomes its own
                project, saved back in a return package.
              </p>
            </div>
          )}
          {doc.bilingual ? (
            <div className="field">
              <span className="field-label">Languages</span>
              <p className="lang-fixed">
                {lang(source).name} <LangChip code={source} /> to{" "}
                {targets[0] ? lang(targets[0]).name : "no target language"}{" "}
                {targets[0] && <LangChip code={targets[0]} />}
              </p>
              <p className="help">
                From the file. A bilingual file is translated into the language
                it was made for.
              </p>
              {targetError && (
                <p className="error" role="alert">
                  {doc.bilingual.targetLang
                    ? targetError
                    : "The file names no target language, so it cannot be translated here."}
                </p>
              )}
            </div>
          ) : (
            <>
              <div className="field">
                <label htmlFor="source">Source language</label>
                <SourceDetection
                  detection={detection!}
                  source={source}
                  touched={sourceTouched}
                  fallback={recentSource}
                />
                <select
                  id="source"
                  value={source}
                  onChange={(e) => {
                    setSource(e.target.value);
                    setSourceTouched(true);
                    setTargets((ts) => ts.filter((t) => t !== e.target.value));
                  }}
                  aria-describedby="source-help"
                >
                  {LANGUAGES.map((l) => (
                    <option key={l.code} value={l.code}>
                      {l.name}
                    </option>
                  ))}
                </select>
                <p id="source-help" className="help">
                  Text shows the language, not the region. Check the variant
                  (for example German (Austria)) if the client needs it.
                </p>
              </div>

              <fieldset className="field">
                <legend>Target languages</legend>
                <TargetPicker
                  source={source}
                  targets={targets}
                  setTargets={setTargets}
                  suggestions={recentTargets}
                />
                {targetError && (
                  <p className="error" role="alert">
                    {targetError}
                  </p>
                )}
              </fieldset>
            </>
          )}

          <div className="field-grid">
            <div className="field">
              <label htmlFor="pname">Project name</label>
              <input
                id="pname"
                value={name}
                onChange={(e) => setName(e.target.value)}
                aria-invalid={!!nameError}
                aria-describedby={nameError ? "pname-err" : undefined}
              />
              {nameError && (
                <p id="pname-err" className="error">
                  {nameError}
                </p>
              )}
            </div>
            <div className="field">
              <label htmlFor="client">
                Client <span className="optional">optional</span>
              </label>
              <input
                id="client"
                list="clients"
                value={client}
                onChange={(e) => setClient(e.target.value)}
                aria-describedby="client-help"
              />
              <datalist id="clients">
                {clients.map((c) => (
                  <option key={c} value={c} />
                ))}
              </datalist>
              <p id="client-help" className="help">
                Used to suggest the client's termbases.
              </p>
            </div>
          </div>

          <div className="panel-actions">
            <button
              type="button"
              className="btn btn-primary"
              disabled={!!targetError || !!nameError}
              onClick={() => setStep(1)}
            >
              Continue to terminology
            </button>
          </div>
        </section>
      )}

      {step === 1 && (
        <section aria-labelledby="step-title" className="panel">
          <h2
            id="step-title"
            ref={stepHeading}
            tabIndex={-1}
            className="panel-title"
          >
            Terminology
          </h2>

          <fieldset className="field">
            <legend>Master termbases to use</legend>
            <p className="help">
              Masters you keep across projects, for a client, a domain or a set
              of languages.
            </p>
            <ul className="choice-list">
              {masters.map((m) => {
                const missing = projectLangs.filter(
                  (l) => !m.languages.includes(l),
                );
                const suggested = !!client && m.client === client;
                const count =
                  (m.extraEntries ?? 0) +
                  state.entries.filter((e) => e.termbaseId === m.id).length;
                return (
                  <li key={m.id}>
                    <label className="choice">
                      <input
                        type="checkbox"
                        checked={selectedMasters.includes(m.id)}
                        onChange={(e) => {
                          setMastersTouched(true);
                          setSelectedMasters((s) =>
                            e.target.checked
                              ? [...s, m.id]
                              : s.filter((x) => x !== m.id),
                          );
                        }}
                      />
                      <span className="choice-body">
                        <span className="choice-title">
                          {m.name}
                          {suggested && (
                            <span className="tag tag-accent">
                              Suggested for {m.client}
                            </span>
                          )}
                        </span>
                        <span className="choice-meta">
                          <LocationBadge
                            location={m.location}
                            server={m.server}
                          />
                          <span>{plural(count, "entry", "entries")}</span>
                          <span className="lang-list">
                            {m.languages.map((l) => (
                              <LangChip key={l} code={l} />
                            ))}
                          </span>
                        </span>
                        {missing.length > 0 &&
                          selectedMasters.includes(m.id) && (
                            <span className="choice-note">
                              <Info size={14} aria-hidden="true" />
                              {missing.map((c) => lang(c).name).join(", ")} will
                              be added to this master.
                            </span>
                          )}
                      </span>
                    </label>
                  </li>
                );
              })}
            </ul>
          </fieldset>

          <fieldset className="field">
            <legend>Where new terms go</legend>
            <ul className="choice-list">
              <li>
                <label
                  className={`choice${selectedMasters.length === 0 ? " is-disabled" : ""}`}
                >
                  <input
                    type="radio"
                    name="mode"
                    value="layered"
                    checked={mode === "layered"}
                    disabled={selectedMasters.length === 0}
                    onChange={() => setMode("layered")}
                  />
                  <span className="choice-body">
                    <span className="choice-title">
                      {w("projectTermbase")}, then merge into the masters{" "}
                      <span className="tag">Recommended</span>
                    </span>
                    <span className="choice-desc">
                      New and changed terms stay in the project. When the
                      project is done, you review them and merge into the
                      masters. The masters stay unchanged until then.
                    </span>
                  </span>
                </label>
              </li>
              <li>
                <label
                  className={`choice${selectedMasters.length === 0 ? " is-disabled" : ""}`}
                >
                  <input
                    type="radio"
                    name="mode"
                    value="direct"
                    checked={mode === "direct"}
                    disabled={selectedMasters.length === 0}
                    onChange={() => setMode("direct")}
                  />
                  <span className="choice-body">
                    <span className="choice-title">Directly into a master</span>
                    <span className="choice-desc">
                      New terms are saved to the master right away. Simple when
                      you work alone on this client.
                    </span>
                    {mode === "direct" && selectedMasters.length > 1 && (
                      <span className="choice-inline">
                        <label htmlFor="direct-target">
                          Master for new terms
                        </label>
                        <select
                          id="direct-target"
                          value={directTarget}
                          onChange={(e) => setDirectTarget(e.target.value)}
                        >
                          {selectedMasters.map((id) => (
                            <option key={id} value={id}>
                              {state.termbases.find((t) => t.id === id)!.name}
                            </option>
                          ))}
                        </select>
                      </span>
                    )}
                  </span>
                </label>
              </li>
              <li>
                <label className="choice">
                  <input
                    type="radio"
                    name="mode"
                    value="own"
                    checked={mode === "own"}
                    onChange={() => setMode("own")}
                  />
                  <span className="choice-body">
                    <span className="choice-title">
                      Own project termbase only
                    </span>
                    <span className="choice-desc">
                      {selectedMasters.length === 0
                        ? "No master selected. "
                        : "Selected masters are not used. "}
                      You can turn it into a master or merge it into one later.
                    </span>
                  </span>
                </label>
              </li>
            </ul>
          </fieldset>

          {mode !== "direct" && (
            <fieldset className="field">
              <legend>Store the project termbase</legend>
              <div
                className="segmented"
                role="radiogroup"
                aria-label="Store the project termbase"
              >
                <label className="segment">
                  <input
                    type="radio"
                    name="loc"
                    checked={location === "local"}
                    onChange={() => {
                      setMastersTouched(true);
                      setLocation("local");
                    }}
                  />
                  <span>On this computer</span>
                </label>
                <label
                  className={`segment${serverAvailable ? "" : " is-disabled"}`}
                >
                  <input
                    type="radio"
                    name="loc"
                    checked={location === "server"}
                    disabled={!serverAvailable}
                    onChange={() => {
                      setMastersTouched(true);
                      setLocation("server");
                    }}
                  />
                  <span>On the server</span>
                </label>
              </div>
              <p className="help">
                {location === "server"
                  ? "Colleagues on the server see the project's terms while you work. Masters stay where they are."
                  : "Only you see the project's terms until you merge. Masters stay where they are."}
              </p>
            </fieldset>
          )}

          {mode === "layered" && (
            <div className="field">
              <label className="check">
                <input
                  type="checkbox"
                  checked={autoUpdate}
                  onChange={(e) => setAutoUpdate(e.target.checked)}
                />
                <span>{w("updateFromMaster")} each time the project opens</span>
              </label>
              <p className="help">
                Otherwise the project keeps the masters as they are today until
                you update it yourself.
              </p>
            </div>
          )}

          <div className="panel-actions">
            <button
              type="button"
              className="btn btn-quiet"
              onClick={() => setStep(0)}
            >
              Back
            </button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => setStep(2)}
            >
              Continue to review
            </button>
          </div>
        </section>
      )}

      {step === 2 && (
        <section aria-labelledby="step-title" className="panel">
          <h2
            id="step-title"
            ref={stepHeading}
            tabIndex={-1}
            className="panel-title"
          >
            Review
          </h2>
          <dl className="summary">
            <dt>Project</dt>
            <dd>
              {name.trim()}
              {client.trim() && (
                <span className="muted"> for {client.trim()}</span>
              )}
            </dd>
            <dt>Document</dt>
            <dd>
              {doc.file}
              {doc.words > 0 && (
                <span className="muted"> ({fmtNum(doc.words)} words)</span>
              )}
            </dd>
            <dt>Languages</dt>
            <dd>
              {lang(source).name} into{" "}
              {targets.map((t) => lang(t).name).join(", ")}
            </dd>
            <dt>Terminology</dt>
            <dd>
              <TerminologySummary
                mode={mode}
                masters={mode === "direct" ? [directTarget] : selectedMasters}
                location={location}
                autoUpdate={autoUpdate}
              />
            </dd>
            <dt>Match analysis</dt>
            <dd>
              {analysis === null ? (
                <p className="muted">Analysing the document…</p>
              ) : analysis.segments === 0 ? (
                <p className="muted">Available once the text can be read.</p>
              ) : (
                <>
                  <p className="help">
                    {analysisMemory.length
                      ? `Against ${analysisMemory.map((m) => m.label).join(" and ")}, counted as in Trados.`
                      : "No translation memory for this client and language pair yet, so only repetitions inside the document count."}
                  </p>
                  <AnalysisTable
                    rows={analysis.rows}
                    words={analysis.words}
                    segments={analysis.segments}
                    caption="Match analysis of the document"
                  />
                </>
              )}
            </dd>
          </dl>
          <div className="panel-actions">
            <button
              type="button"
              className="btn btn-quiet"
              onClick={() => setStep(1)}
            >
              Back
            </button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={create}
              disabled={creating}
            >
              {creating ? "Reading the document…" : "Create project"}
            </button>
          </div>
        </section>
      )}
    </div>
  );
}

function TerminologySummary({
  mode,
  masters,
  location,
  autoUpdate,
}: {
  mode: SetupMode;
  masters: string[];
  location: Location;
  autoUpdate: boolean;
}) {
  const { state } = useStore();
  const w = useWords();
  const names = masters
    .map((id) => state.termbases.find((t) => t.id === id)?.name)
    .filter(Boolean)
    .join(", ");
  if (mode === "direct") return <>New terms go straight into {names}.</>;
  if (mode === "own")
    return (
      <>
        A new {w("projectTermbase").toLowerCase()}{" "}
        <LocationBadge location={location} />, no master.
      </>
    );
  return (
    <>
      A new {w("projectTermbase").toLowerCase()}{" "}
      <LocationBadge location={location} />, {w("basedOn").toLowerCase()}{" "}
      {names}.{" "}
      {autoUpdate
        ? "Updates from the masters when the project opens."
        : "Keeps today's state of the masters."}
    </>
  );
}

function SourceDetection({
  detection,
  source,
  touched,
  fallback,
}: {
  detection: Detection;
  source: LangCode;
  touched: boolean;
  fallback: LangCode;
}) {
  if (touched) {
    return (
      <p className="detect detect-info">
        <Info size={16} aria-hidden="true" />
        <span>Set by you: {lang(source).name}.</span>
      </p>
    );
  }
  if (detection.reason === "empty") {
    return (
      <p className="detect detect-warn">
        <AlertTriangle size={16} aria-hidden="true" />
        <span>
          No text found in this file (for example a scanned PDF), so the
          language can't be detected. Preselected {lang(fallback).name}, the
          source language of most of your projects. Change it if it's wrong.
        </span>
      </p>
    );
  }
  if (detection.reason === "short" || !detection.code) {
    return (
      <p className="detect detect-warn">
        <AlertTriangle size={16} aria-hidden="true" />
        <span>
          Too little text to tell ({plural(detection.basis, "word", "words")}).
          Preselected {lang(fallback).name}, the source language of most of your
          projects.
        </span>
      </p>
    );
  }
  const pct = Math.round(detection.confidence * 100);
  if (detection.confidence < CONFIDENT) {
    const alt = detection.alternatives
      .map((a) => `${lang(a.code).name} ${Math.round(a.confidence * 100)}%`)
      .join(", ");
    return (
      <p className="detect detect-warn">
        <AlertTriangle size={16} aria-hidden="true" />
        <span>
          Probably {lang(detection.code).name} ({pct}%)
          {alt ? `, but it could also be ${alt}` : ""}. Please check.
        </span>
      </p>
    );
  }
  return (
    <p className="detect detect-ok">
      <CheckCircle2 size={16} aria-hidden="true" />
      <span>
        Detected {lang(detection.code).name}, {pct}% confident, from{" "}
        {detection.unit === "characters"
          ? plural(detection.basis, "character", "characters")
          : plural(detection.basis, "word", "words")}{" "}
        of the document.
      </span>
    </p>
  );
}

function TargetPicker({
  source,
  targets,
  setTargets,
  suggestions,
}: {
  source: LangCode;
  targets: LangCode[];
  setTargets: (f: (t: LangCode[]) => LangCode[]) => void;
  suggestions: LangCode[];
}) {
  const [adding, setAdding] = useState("");
  const quick = [...new Set([...suggestions, ...targets])].filter(
    (c) => c !== source,
  );
  const others = LANGUAGES.filter(
    (l) => l.code !== source && !quick.includes(l.code),
  );
  return (
    <div className="targets">
      <ul className="target-list">
        {quick.map((c) => (
          <li key={c}>
            <label className="check">
              <input
                type="checkbox"
                checked={targets.includes(c)}
                onChange={(e) =>
                  setTargets((t) =>
                    e.target.checked ? [...t, c] : t.filter((x) => x !== c),
                  )
                }
              />
              <span>{lang(c).name}</span>
            </label>
          </li>
        ))}
      </ul>
      <div className="add-lang">
        <label htmlFor="add-lang" className="sr-only">
          Add another target language
        </label>
        <select
          id="add-lang"
          value={adding}
          onChange={(e) => setAdding(e.target.value)}
        >
          <option value="">Add another language…</option>
          {others.map((l) => (
            <option key={l.code} value={l.code}>
              {l.name}
            </option>
          ))}
        </select>
        <button
          type="button"
          className="btn btn-small"
          disabled={!adding}
          onClick={() => {
            setTargets((t) => [...t, adding]);
            setAdding("");
          }}
        >
          <Plus size={14} aria-hidden="true" />
          Add
        </button>
      </div>
      {targets.some((t) => !quick.includes(t)) && (
        <ul className="target-chips">
          {targets
            .filter((t) => !quick.includes(t))
            .map((t) => (
              <li key={t}>
                {lang(t).name}
                <button
                  type="button"
                  className="icon-button"
                  aria-label={`Remove ${lang(t).name}`}
                  onClick={() => setTargets((x) => x.filter((y) => y !== t))}
                >
                  <X size={14} aria-hidden="true" />
                </button>
              </li>
            ))}
        </ul>
      )}
    </div>
  );
}

function ChooseDocument({ onChosen }: { onChosen: (d: PendingDoc) => void }) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <div className="page page-narrow">
      <PageHeader title="New project" />
      <section className="dropzone" aria-labelledby="dz2">
        <div className="dropzone-text">
          <h2 id="dz2">Choose the document to translate</h2>
          <p>
            Interlinea detects its source language. Word documents (.docx) go
            through the core's file filter, so the translation keeps their
            formatting. Bilingual files from Trados (.sdlxliff), memoQ
            (.mqxliff) and other tools (.xlf, .xliff) open with their
            translations and states and are saved back for that tool. Plain text
            formats are read as text; other formats fall back to your usual
            source language.
          </p>
        </div>
        <button
          type="button"
          className="btn btn-primary"
          onClick={() => input.current?.click()}
        >
          Choose document
        </button>
        <input
          ref={input}
          type="file"
          hidden
          onChange={async (e) => {
            const f = e.target.files?.[0];
            if (!f) return;
            onChosen(await readDocument(f));
          }}
        />
      </section>
    </div>
  );
}

/** A language code as the prototype lists it (de-de becomes de-DE). */
function knownLang(code: string): LangCode {
  const lower = code.toLowerCase().replace("_", "-");
  const exact = LANGUAGES.find((l) => l.code.toLowerCase() === lower);
  if (exact) return exact.code;
  if (!lower.includes("-")) {
    const byPrimary = LANGUAGES.find((l) => primary(l.code) === lower);
    if (byPrimary) return byPrimary.code;
  }
  const [p, r] = lower.split("-");
  return r ? `${p}-${r.toUpperCase()}` : lower;
}

function bilingualSummary(segments: Segment[]): string {
  const done = segments.filter((s) => s.status === "confirmed").length;
  const locked = segments.filter((s) => s.locked).length;
  return `${done} of ${plural(segments.length, "segment", "segments")} confirmed${locked ? `, ${locked} locked` : ""}`;
}

function mostCommon<T>(xs: T[]): T | undefined {
  const m = new Map<T, number>();
  for (const x of xs) m.set(x, (m.get(x) ?? 0) + 1);
  return [...m.entries()].sort((a, b) => b[1] - a[1])[0]?.[0];
}
