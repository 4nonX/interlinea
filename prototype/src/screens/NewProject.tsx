import { useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, CheckCircle2, FileText, Info, Plus, X } from "lucide-react";
import { LANGUAGES, SAMPLE_DOCS, lang, primary, type LangCode, type Location, type SetupMode } from "../data";
import { CONFIDENT, detectLanguage, readText, type Detection } from "../detect";
import { countWords, takePendingDoc, type PendingDoc } from "../draft";
import { useStore, useWords } from "../store";
import { fmtNum, plural } from "../format";
import { LangChip, LocationBadge, PageHeader } from "../components/ui";
import { go } from "../App";

const STEPS = ["Languages", "Terminology", "Review"] as const;

function nameFromFile(file: string): string {
  return file.replace(/\.[^.]+$/, "").replace(/[_]+/g, " ").trim();
}

export function NewProject({ sample }: { sample?: number }) {
  const { state, dispatch, notify } = useStore();
  const w = useWords();
  const [doc, setDoc] = useState<PendingDoc | null>(() => {
    const pending = takePendingDoc();
    if (pending) return pending;
    // Deep link to a sample document: #/new/sample/<index>
    const s = sample !== undefined ? SAMPLE_DOCS[sample] : undefined;
    return s ? { file: s.file, words: s.words, text: s.text || null, client: s.client } : null;
  });
  const [step, setStep] = useState(0);
  const stepHeading = useRef<HTMLHeadingElement>(null);

  // Most frequent source and target languages of earlier projects: the fallback
  // when a document gives no usable text.
  const recentSource = useMemo(() => mostCommon(state.projects.map((p) => p.sourceLang)) ?? "de-DE", [state.projects]);
  const recentTargets = useMemo(() => [...new Set(state.projects.flatMap((p) => p.targetLangs))], [state.projects]);
  const clients = useMemo(() => [...new Set(state.termbases.map((t) => t.client).filter(Boolean) as string[])], [state.termbases]);

  const detection: Detection | null = useMemo(() => (doc ? detectLanguage(doc.text ?? "") : null), [doc]);
  const [source, setSource] = useState<LangCode>("");
  const [sourceTouched, setSourceTouched] = useState(false);
  const [targets, setTargets] = useState<LangCode[]>([]);
  const [name, setName] = useState("");
  const [client, setClient] = useState("");

  useEffect(() => {
    if (!doc) return;
    setName(nameFromFile(doc.file));
    setClient(doc.client ?? "");
    const detected = detection?.code ?? null;
    const src = detected ?? recentSource;
    setSource(src);
    setSourceTouched(false);
    const suggested = recentTargets.filter((t) => primary(t) !== primary(src));
    setTargets(suggested.length ? [suggested[0]!] : [primary(src) === "en" ? "de-DE" : "en-GB"]);
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
    const byClient = masters.filter((m) => client && m.client === client).map((m) => m.id);
    setSelectedMasters(byClient);
  }, [client, mastersTouched]);

  useEffect(() => {
    if (selectedMasters.length === 0 && mode !== "own") setMode("own");
    if (selectedMasters.length > 0 && mode === "own" && !mastersTouched) setMode("layered");
    if (!selectedMasters.includes(directTarget)) setDirectTarget(selectedMasters[0] ?? "");
    const serverMaster = state.termbases.find((t) => selectedMasters.includes(t.id) && t.location === "server");
    if (!mastersTouched) setLocation(serverMaster ? "server" : "local");
  }, [selectedMasters]);

  useEffect(() => {
    stepHeading.current?.focus();
  }, [step]);

  const serverAvailable = state.termbases.some((t) => t.location === "server");
  const projectLangs = [source, ...targets];
  const targetError = targets.length === 0 ? "Choose at least one target language." : targets.includes(source) ? "A target language can't be the same as the source language." : null;
  const nameError = name.trim() ? null : "Give the project a name.";

  function create() {
    const id = `p-${Date.now().toString(36)}`;
    const usedMasters = mode === "direct" ? [directTarget] : mode === "layered" ? selectedMasters : [];
    dispatch({
      type: "createProject",
      id,
      now: new Date().toISOString(),
      input: { name: name.trim(), file: doc!.file, words: doc!.words, sourceLang: source, targetLangs: targets, client: client.trim() || undefined, mode, masters: usedMasters, location, text: doc!.text },
    });
    notify(`Created project “${name.trim()}”`);
    go({ page: "editor", projectId: id });
  }

  if (!doc) return <ChooseDocument onChosen={setDoc} />;

  return (
    <div className="page page-narrow">
      <PageHeader title="New project" description={<>From <strong>{doc.file}</strong></>}>
        <a className="btn btn-quiet" href="#/projects">
          Cancel
        </a>
      </PageHeader>

      <ol className="stepper" aria-label="Steps">
        {STEPS.map((s, i) => (
          <li key={s} aria-current={i === step ? "step" : undefined} className={i < step ? "is-done" : undefined}>
            <span className="stepper-num" aria-hidden="true">
              {i < step ? <CheckCircle2 size={16} /> : i + 1}
            </span>
            {i < step ? (
              <button type="button" className="link-button" onClick={() => setStep(i)}>
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
          <h2 id="step-title" ref={stepHeading} tabIndex={-1} className="panel-title">
            Languages
          </h2>

          <div className="file-row">
            <FileText size={20} aria-hidden="true" />
            <div>
              <div className="cell-title">{doc.file}</div>
              <div className="cell-sub">{doc.words ? plural(doc.words, "word", "words") : "Word count after import"}</div>
            </div>
            <button type="button" className="btn btn-quiet btn-small file-row-action" onClick={() => setDoc(null)}>
              Choose another file
            </button>
          </div>

          <div className="field">
            <label htmlFor="source">Source language</label>
            <SourceDetection detection={detection!} source={source} touched={sourceTouched} fallback={recentSource} />
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
              Text shows the language, not the region. Check the variant (for example German (Austria)) if the client needs it.
            </p>
          </div>

          <fieldset className="field">
            <legend>Target languages</legend>
            <TargetPicker source={source} targets={targets} setTargets={setTargets} suggestions={recentTargets} />
            {targetError && (
              <p className="error" role="alert">
                {targetError}
              </p>
            )}
          </fieldset>

          <div className="field-grid">
            <div className="field">
              <label htmlFor="pname">Project name</label>
              <input id="pname" value={name} onChange={(e) => setName(e.target.value)} aria-invalid={!!nameError} aria-describedby={nameError ? "pname-err" : undefined} />
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
              <input id="client" list="clients" value={client} onChange={(e) => setClient(e.target.value)} aria-describedby="client-help" />
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
            <button type="button" className="btn btn-primary" disabled={!!targetError || !!nameError} onClick={() => setStep(1)}>
              Continue to terminology
            </button>
          </div>
        </section>
      )}

      {step === 1 && (
        <section aria-labelledby="step-title" className="panel">
          <h2 id="step-title" ref={stepHeading} tabIndex={-1} className="panel-title">
            Terminology
          </h2>

          <fieldset className="field">
            <legend>Master termbases to use</legend>
            <p className="help">Masters you keep across projects, for a client, a domain or a set of languages.</p>
            <ul className="choice-list">
              {masters.map((m) => {
                const missing = projectLangs.filter((l) => !m.languages.includes(l));
                const suggested = !!client && m.client === client;
                const count = (m.extraEntries ?? 0) + state.entries.filter((e) => e.termbaseId === m.id).length;
                return (
                  <li key={m.id}>
                    <label className="choice">
                      <input
                        type="checkbox"
                        checked={selectedMasters.includes(m.id)}
                        onChange={(e) => {
                          setMastersTouched(true);
                          setSelectedMasters((s) => (e.target.checked ? [...s, m.id] : s.filter((x) => x !== m.id)));
                        }}
                      />
                      <span className="choice-body">
                        <span className="choice-title">
                          {m.name}
                          {suggested && <span className="tag tag-accent">Suggested for {m.client}</span>}
                        </span>
                        <span className="choice-meta">
                          <LocationBadge location={m.location} server={m.server} />
                          <span>{plural(count, "entry", "entries")}</span>
                          <span className="lang-list">
                            {m.languages.map((l) => (
                              <LangChip key={l} code={l} />
                            ))}
                          </span>
                        </span>
                        {missing.length > 0 && selectedMasters.includes(m.id) && (
                          <span className="choice-note">
                            <Info size={14} aria-hidden="true" />
                            {missing.map((c) => lang(c).name).join(", ")} will be added to this master.
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
                <label className={`choice${selectedMasters.length === 0 ? " is-disabled" : ""}`}>
                  <input type="radio" name="mode" value="layered" checked={mode === "layered"} disabled={selectedMasters.length === 0} onChange={() => setMode("layered")} />
                  <span className="choice-body">
                    <span className="choice-title">
                      {w("projectTermbase")}, then merge into the masters <span className="tag">Recommended</span>
                    </span>
                    <span className="choice-desc">
                      New and changed terms stay in the project. When the project is done, you review them and merge into the masters. The masters stay unchanged until then.
                    </span>
                  </span>
                </label>
              </li>
              <li>
                <label className={`choice${selectedMasters.length === 0 ? " is-disabled" : ""}`}>
                  <input type="radio" name="mode" value="direct" checked={mode === "direct"} disabled={selectedMasters.length === 0} onChange={() => setMode("direct")} />
                  <span className="choice-body">
                    <span className="choice-title">Directly into a master</span>
                    <span className="choice-desc">New terms are saved to the master right away. Simple when you work alone on this client.</span>
                    {mode === "direct" && selectedMasters.length > 1 && (
                      <span className="choice-inline">
                        <label htmlFor="direct-target">Master for new terms</label>
                        <select id="direct-target" value={directTarget} onChange={(e) => setDirectTarget(e.target.value)}>
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
                  <input type="radio" name="mode" value="own" checked={mode === "own"} onChange={() => setMode("own")} />
                  <span className="choice-body">
                    <span className="choice-title">Own project termbase only</span>
                    <span className="choice-desc">
                      {selectedMasters.length === 0 ? "No master selected. " : "Selected masters are not used. "}
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
              <div className="segmented" role="radiogroup" aria-label="Store the project termbase">
                <label className="segment">
                  <input type="radio" name="loc" checked={location === "local"} onChange={() => { setMastersTouched(true); setLocation("local"); }} />
                  <span>On this computer</span>
                </label>
                <label className={`segment${serverAvailable ? "" : " is-disabled"}`}>
                  <input type="radio" name="loc" checked={location === "server"} disabled={!serverAvailable} onChange={() => { setMastersTouched(true); setLocation("server"); }} />
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
                <input type="checkbox" checked={autoUpdate} onChange={(e) => setAutoUpdate(e.target.checked)} />
                <span>{w("updateFromMaster")} each time the project opens</span>
              </label>
              <p className="help">Otherwise the project keeps the masters as they are today until you update it yourself.</p>
            </div>
          )}

          <div className="panel-actions">
            <button type="button" className="btn btn-quiet" onClick={() => setStep(0)}>
              Back
            </button>
            <button type="button" className="btn btn-primary" onClick={() => setStep(2)}>
              Continue to review
            </button>
          </div>
        </section>
      )}

      {step === 2 && (
        <section aria-labelledby="step-title" className="panel">
          <h2 id="step-title" ref={stepHeading} tabIndex={-1} className="panel-title">
            Review
          </h2>
          <dl className="summary">
            <dt>Project</dt>
            <dd>
              {name.trim()}
              {client.trim() && <span className="muted"> for {client.trim()}</span>}
            </dd>
            <dt>Document</dt>
            <dd>
              {doc.file}
              {doc.words > 0 && <span className="muted"> ({fmtNum(doc.words)} words)</span>}
            </dd>
            <dt>Languages</dt>
            <dd>
              {lang(source).name} into {targets.map((t) => lang(t).name).join(", ")}
            </dd>
            <dt>Terminology</dt>
            <dd>
              <TerminologySummary mode={mode} masters={mode === "direct" ? [directTarget] : selectedMasters} location={location} autoUpdate={autoUpdate} />
            </dd>
          </dl>
          <div className="panel-actions">
            <button type="button" className="btn btn-quiet" onClick={() => setStep(1)}>
              Back
            </button>
            <button type="button" className="btn btn-primary" onClick={create}>
              Create project
            </button>
          </div>
        </section>
      )}
    </div>
  );
}

function TerminologySummary({ mode, masters, location, autoUpdate }: { mode: SetupMode; masters: string[]; location: Location; autoUpdate: boolean }) {
  const { state } = useStore();
  const w = useWords();
  const names = masters.map((id) => state.termbases.find((t) => t.id === id)?.name).filter(Boolean).join(", ");
  if (mode === "direct") return <>New terms go straight into {names}.</>;
  if (mode === "own")
    return (
      <>
        A new {w("projectTermbase").toLowerCase()} <LocationBadge location={location} />, no master.
      </>
    );
  return (
    <>
      A new {w("projectTermbase").toLowerCase()} <LocationBadge location={location} />, {w("basedOn").toLowerCase()} {names}. {autoUpdate ? "Updates from the masters when the project opens." : "Keeps today's state of the masters."}
    </>
  );
}

function SourceDetection({ detection, source, touched, fallback }: { detection: Detection; source: LangCode; touched: boolean; fallback: LangCode }) {
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
          No text found in this file (for example a scanned PDF), so the language can't be detected. Preselected {lang(fallback).name}, the source language of most of your projects. Change it if it's wrong.
        </span>
      </p>
    );
  }
  if (detection.reason === "short" || !detection.code) {
    return (
      <p className="detect detect-warn">
        <AlertTriangle size={16} aria-hidden="true" />
        <span>
          Too little text to tell ({plural(detection.basis, "word", "words")}). Preselected {lang(fallback).name}, the source language of most of your projects.
        </span>
      </p>
    );
  }
  const pct = Math.round(detection.confidence * 100);
  if (detection.confidence < CONFIDENT) {
    const alt = detection.alternatives.map((a) => `${lang(a.code).name} ${Math.round(a.confidence * 100)}%`).join(", ");
    return (
      <p className="detect detect-warn">
        <AlertTriangle size={16} aria-hidden="true" />
        <span>
          Probably {lang(detection.code).name} ({pct}%){alt ? `, but it could also be ${alt}` : ""}. Please check.
        </span>
      </p>
    );
  }
  return (
    <p className="detect detect-ok">
      <CheckCircle2 size={16} aria-hidden="true" />
      <span>
        Detected {lang(detection.code).name}, {pct}% confident, from {detection.unit === "characters" ? plural(detection.basis, "character", "characters") : plural(detection.basis, "word", "words")} of the document.
      </span>
    </p>
  );
}

function TargetPicker({ source, targets, setTargets, suggestions }: { source: LangCode; targets: LangCode[]; setTargets: (f: (t: LangCode[]) => LangCode[]) => void; suggestions: LangCode[] }) {
  const [adding, setAdding] = useState("");
  const quick = [...new Set([...suggestions, ...targets])].filter((c) => c !== source);
  const others = LANGUAGES.filter((l) => l.code !== source && !quick.includes(l.code));
  return (
    <div className="targets">
      <ul className="target-list">
        {quick.map((c) => (
          <li key={c}>
            <label className="check">
              <input type="checkbox" checked={targets.includes(c)} onChange={(e) => setTargets((t) => (e.target.checked ? [...t, c] : t.filter((x) => x !== c)))} />
              <span>{lang(c).name}</span>
            </label>
          </li>
        ))}
      </ul>
      <div className="add-lang">
        <label htmlFor="add-lang" className="sr-only">
          Add another target language
        </label>
        <select id="add-lang" value={adding} onChange={(e) => setAdding(e.target.value)}>
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
                <button type="button" className="icon-button" aria-label={`Remove ${lang(t).name}`} onClick={() => setTargets((x) => x.filter((y) => y !== t))}>
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
          <p>Interlinea detects its source language. Plain text formats are read in this prototype; other formats fall back to your usual source language.</p>
        </div>
        <button type="button" className="btn btn-primary" onClick={() => input.current?.click()}>
          Choose document
        </button>
        <input
          ref={input}
          type="file"
          hidden
          onChange={async (e) => {
            const f = e.target.files?.[0];
            if (!f) return;
            const text = await readText(f);
            onChosen({ file: f.name, words: text ? countWords(text) : 0, text });
          }}
        />
      </section>
    </div>
  );
}

function mostCommon<T>(xs: T[]): T | undefined {
  const m = new Map<T, number>();
  for (const x of xs) m.set(x, (m.get(x) ?? 0) + 1);
  return [...m.entries()].sort((a, b) => b[1] - a[1])[0]?.[0];
}
