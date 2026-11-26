import { useEffect, useMemo, useRef, useState, type DragEvent } from "react";
import {
  AlertTriangle,
  CheckCircle2,
  FileText,
  FolderOpen,
  Info,
  Upload,
  X,
} from "lucide-react";
import {
  DEFAULT_PREPARE,
  DEFAULT_QA,
  LANGUAGES,
  lang,
  primary,
  projectMemoryName,
  type FileRole,
  type LangCode,
  type Project,
  type ProjectFile,
  type TranslationMemory,
} from "../data";
import {
  CONFIDENT,
  detectLanguage,
  readFiles,
  type Detection,
} from "../detect";
import {
  core,
  DOCUMENT_FORMAT_KEEPS,
  DOCUMENT_FORMAT_NAME,
  documentFormat,
  loadCore,
  packageKind,
  XLIFF_FORMAT_NAME,
  type Extracted,
} from "../core";
import {
  buildFiles,
  docName,
  docTarget,
  draftDocs,
  extractWordDocs,
  kindOf,
  knownLang,
  readable,
  type DraftDoc,
} from "../setup";
import { openRefs, persistMemory } from "../editor/memory";
import { AnalysisTable } from "../components/AnalysisTable";
import { takePendingDocs, type PendingDoc } from "../draft";
import { useLocalStore } from "../store";
import { fmtDate, fmtNum, plural } from "../format";
import { LangChip, PageHeader } from "../components/ui";
import { go } from "../App";
import { errorText, remoteId, useRemote } from "../remote";
import { publish } from "../publish";
import {
  analyzeFiles,
  draftOf,
  draftOfTemplate,
  memoryRefs,
  pretranslateFiles,
  problemsAt,
  templateOf,
  validate,
  type CoreReport,
  type Draft,
  type Problem,
} from "../project";
import {
  ChecksSection,
  checksOn,
  withDictionaries,
  MemoriesSection,
  PrepareSection,
  SegmentationSection,
  Problems,
  TargetPicker,
  TerminologySection,
  newSetting,
  type NewMemory,
} from "./ProjectSections";
import { droppedFiles, chosenFiles } from "../files";

const STEPS = [
  "Start",
  "Files and languages",
  "Memories",
  "Terminology",
  "Review",
] as const;

/** The fields each step sets, for its problems and its Continue button. */
const STEP_FIELDS: string[][] = [
  [],
  ["name", "client", "due", "sourceLang", "targetLangs", "files"],
  ["memories"],
  ["terminology"],
  [],
];

type Start =
  | { kind: "blank" }
  | { kind: "template"; id: string }
  | { kind: "project"; id: string };

function nameFromFile(file: string): string {
  return file
    .replace(/^.*\//, "")
    .replace(/\.[^.]+$/, "")
    .replace(/[_]+/g, " ")
    .trim();
}

function emptyDraft(contextRule: Draft["context"]): Draft {
  return {
    name: "",
    client: "",
    description: "",
    due: "",
    sourceLang: "",
    targetLangs: [],
    files: [],
    memories: {},
    projectMemory: true,
    terminology: {
      mode: "own",
      masters: [],
      addLanguages: [],
      location: "local",
      autoUpdate: true,
    },
    context: contextRule,
    qa: DEFAULT_QA,
    prepare: DEFAULT_PREPARE,
    segmentation: [],
  };
}

export function NewProject() {
  const { state, dispatch, notify } = useLocalStore();
  const [docs, setDocs] = useState<DraftDoc[]>(() => {
    const pending = takePendingDocs();
    return draftDocs(pending);
  });
  const [step, setStep] = useState(0);
  const stepHeading = useRef<HTMLHeadingElement>(null);
  const [draft, setDraft] = useState<Draft>(() =>
    emptyDraft(state.contextRule),
  );
  const [start, setStart] = useState<Start>({ kind: "blank" });
  const [touched, setTouched] = useState({
    name: false,
    source: false,
    targets: false,
    masters: false,
    memories: false,
    checks: false,
  });
  const touch = (k: keyof typeof touched) =>
    setTouched((t) => (t[k] ? t : { ...t, [k]: true }));
  const [created, setCreated] = useState<NewMemory[]>([]);
  const [asTemplate, setAsTemplate] = useState(false);
  const [templateName, setTemplateName] = useState("");
  const [creating, setCreating] = useState(false);
  const remote = useRemote();
  // Where the project is kept: here, or shared on the team server.
  const [where, setWhere] = useState<"here" | "server">("here");
  const [publishProblem, setPublishProblem] = useState("");

  const [c, setC] = useState(() => core());
  useEffect(() => {
    if (c) return;
    let live = true;
    loadCore().then(
      (x) => live && setC(x),
      (e: unknown) =>
        live &&
        notify(
          `The core could not be loaded: ${e instanceof Error ? e.message : String(e)}`,
        ),
    );
    return () => {
      live = false;
    };
  }, [c, notify]);

  useEffect(() => {
    stepHeading.current?.focus();
  }, [step]);

  // Earlier projects suggest the languages when the files give none.
  const recentSource = useMemo(
    () => mostCommon(state.projects.map((p) => p.sourceLang)) ?? "de-DE",
    [state.projects],
  );
  const recentTargets = useMemo(
    () => [...new Set(state.projects.flatMap((p) => p.targetLangs))],
    [state.projects],
  );
  const clients = useMemo(
    () =>
      [
        ...new Set(
          [
            ...state.termbases.map((t) => t.client),
            ...state.memories.map((m) => m.client),
            ...state.projects.map((p) => p.client),
          ].filter(Boolean) as string[],
        ),
      ].sort(),
    [state.termbases, state.memories, state.projects],
  );

  const translatable = docs.filter((d) => d.role === "translatable");
  const bilingual = translatable.filter((d) => d.doc.bilingual);
  const bilingualSources = [
    ...new Set(
      bilingual
        .map((d) => d.doc.bilingual!.sourceLang)
        .filter(Boolean)
        .map((l) => knownLang(l)),
    ),
  ];
  const fixedTargets = [
    ...new Set(
      bilingual
        .map((d) => d.doc.bilingual!.targetLang)
        .filter(Boolean)
        .map((t) => knownLang(t!)),
    ),
  ];
  const packages = [
    ...new Set(docs.map((d) => d.doc.sdlPackage?.name).filter(Boolean)),
  ];
  const detection: Detection = useMemo(
    () =>
      detectLanguage(
        translatable
          // A bilingual file that does not say its source language is
          // read like any other text.
          .filter((d) => !d.doc.bilingual?.sourceLang)
          .map((d) => d.doc.text ?? "")
          .join("\n")
          .slice(0, 20000),
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [docs],
  );

  // Suggestions follow the files and the start until the user sets a field.
  useEffect(() => {
    setDraft((d) => {
      let next = d;
      if (!touched.name) {
        const first = docs[0];
        const name = first
          ? nameFromFile(
              first.doc.sdlPackage?.name ?? first.doc.path ?? first.doc.file,
            )
          : "";
        if (name !== d.name) next = { ...next, name };
      }
      if (bilingualSources.length === 1) {
        if (next.sourceLang !== bilingualSources[0])
          next = { ...next, sourceLang: bilingualSources[0]! };
      } else if (!touched.source && start.kind === "blank") {
        const src = detection.code ?? recentSource;
        if (next.sourceLang !== src) next = { ...next, sourceLang: src };
      }
      let targets = next.targetLangs.filter((t) => t !== next.sourceLang);
      for (const t of fixedTargets)
        if (!targets.includes(t)) targets = [...targets, t];
      if (
        !touched.targets &&
        start.kind === "blank" &&
        fixedTargets.length === 0
      ) {
        const suggested = recentTargets.filter(
          (t) => primary(t) !== primary(next.sourceLang),
        );
        targets = suggested.length
          ? [suggested[0]!]
          : [primary(next.sourceLang) === "en" ? "de-DE" : "en-GB"];
      }
      if (targets.join() !== next.targetLangs.join())
        next = { ...next, targetLangs: targets };
      return next;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [docs, detection, start, touched.name, touched.source, touched.targets]);

  // Memory settings exist only for the project's target languages; the
  // client's memories that fit are suggested until the user edits them.
  useEffect(() => {
    setDraft((d) => {
      const memories: Draft["memories"] = {};
      for (const t of d.targetLangs) {
        memories[t] = touched.memories
          ? (d.memories[t] ?? [])
          : state.memories
              .filter(
                (m) =>
                  !m.projectId &&
                  !!d.client &&
                  m.client === d.client &&
                  primary(m.sourceLang) === primary(d.sourceLang) &&
                  primary(m.targetLang) === primary(t),
              )
              .map((m) => newSetting(m.name, d.projectMemory));
      }
      const same =
        Object.keys(memories).length === Object.keys(d.memories).length &&
        Object.entries(memories).every(
          ([k, v]) => JSON.stringify(v) === JSON.stringify(d.memories[k]),
        );
      return same ? d : { ...d, memories };
    });
  }, [
    draft.targetLangs,
    draft.sourceLang,
    draft.client,
    draft.projectMemory,
    touched.memories,
    state.memories,
  ]);

  // Each target language spells with the dictionary that fits it, until
  // the user sets the checks; languages the project lost lose theirs.
  useEffect(() => {
    setDraft((d) => {
      const kept = Object.fromEntries(
        Object.entries(d.qa.dictionaries).filter(([l]) =>
          d.targetLangs.includes(l),
        ),
      );
      let qa = { ...d.qa, dictionaries: kept };
      if (!touched.checks)
        qa = withDictionaries(qa, d.targetLangs, state.dictionaries);
      return JSON.stringify(qa) === JSON.stringify(d.qa) ? d : { ...d, qa };
    });
  }, [draft.targetLangs, touched.checks, state.dictionaries]);

  // Masters for the client are suggested until the user chooses.
  useEffect(() => {
    if (touched.masters || start.kind !== "blank") return;
    setDraft((d) => {
      const masters = state.termbases
        .filter(
          (t) => t.kind === "master" && !!d.client && t.client === d.client,
        )
        .map((t) => t.id);
      const mode = masters.length ? "layered" : "own";
      const t = d.terminology;
      if (
        t.masters.join() === masters.join() && t.mode === mode)
        return d;
      return {
        ...d,
        terminology: {
          ...t,
          masters,
          mode,
          addLanguages: [],
          writeTo: undefined,
        },
      };
    });
  }, [draft.client, touched.masters, start, state.termbases]);

  function applyStart(s: Start) {
    setStart(s);
    if (s.kind === "blank") return;
    const tpl =
      s.kind === "template"
        ? state.templates.find((t) => t.id === s.id)
        : undefined;
    const earlier =
      s.kind === "project"
        ? state.projects.find((p) => p.id === s.id)
        : undefined;
    const from = tpl ? draftOfTemplate(tpl) : earlier ? draftOf(earlier) : null;
    if (!from) return;
    setDraft((d) => ({
      ...d,
      ...from,
      name: d.name,
      due: "",
      description: "",
      // The memories of the earlier project, but not its own project memory.
      memories: from.memories,
      terminology: { ...from.terminology, addLanguages: [] },
    }));
    setTouched((t) => ({
      ...t,
      source: true,
      targets: true,
      masters: true,
      memories: true,
    }));
  }

  const files: Draft["files"] = docs.map((d) => ({
    name: docName(d),
    role: d.role,
    targetLang: docTarget(d),
  }));
  const full: Draft = { ...draft, files };

  // The setup checks of the core, and the few that need what the page holds.
  const problems: Problem[] = useMemo(() => {
    if (!c) return [];
    const ps = validate(c, full, {
      memories: [...state.memories, ...created.map((n) => n.memory)],
      termbases: state.termbases,
      dictionaries: state.dictionaries,
    });
    if (
      state.projects.some(
        (p) => p.name.toLowerCase() === full.name.trim().toLowerCase(),
      )
    )
      ps.push({
        field: "name",
        message: "A project with this name exists already.",
      });
    if (bilingualSources.length > 1)
      ps.push({
        field: "files",
        message: `The bilingual files have different source languages (${bilingualSources.map((l) => lang(l).name).join(", ")}). A project has one source language.`,
      });
    if (packages.length > 1)
      ps.push({
        field: "files",
        message:
          "A project holds the files of one package, so it can be saved back as one return package.",
      });
    if (asTemplate && step === 4) {
      if (!templateName.trim())
        ps.push({ field: "template", message: "Give the template a name." });
      else if (
        state.templates.some(
          (t) => t.name.toLowerCase() === templateName.trim().toLowerCase(),
        )
      )
        ps.push({
          field: "template",
          message: "A template with this name exists already.",
        });
    }
    return ps;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    c,
    JSON.stringify(full),
    state.memories,
    state.termbases,
    state.dictionaries,
    state.projects,
    state.templates,
    created,
    asTemplate,
    templateName,
    step,
  ]);

  const stepProblems = (i: number) =>
    STEP_FIELDS[i]!.flatMap((f) => problemsAt(problems, f));

  // Word files are extracted again with the confirmed source language, so
  // sentences break as that language does; the review and the project use it.
  // Only rules that are filled in cut anything.
  const rulesKey = JSON.stringify(draft.segmentation.filter((r) => r.before || r.after));
  const [words, setWords] = useState<{
    rules: string;
    lang: LangCode;
    docs: DraftDoc[];
    ex: Map<string, Extracted>;
  } | null>(null);
  const [prepareError, setPrepareError] = useState("");
  useEffect(() => {
    if (step < 4 || !draft.sourceLang) return;
    if (words && words.lang === draft.sourceLang && words.docs === docs && words.rules === rulesKey) return;
    let live = true;
    setPrepareError("");
    extractWordDocs(docs, draft.sourceLang, draft.segmentation).then(
      (ex) => live && setWords({ lang: draft.sourceLang, docs, ex, rules: rulesKey }),
      (e: unknown) =>
        live && setPrepareError(e instanceof Error ? e.message : String(e)),
    );
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step, draft.sourceLang, docs, rulesKey]);

  const projectFiles: ProjectFile[] | null = useMemo(
    () =>
      words && words.lang === draft.sourceLang && words.docs === docs && words.rules === rulesKey
        ? buildFiles(docs, words.ex, draft.targetLangs, (i) => `f${i + 1}`)
        : null,
    [docs, words, draft.sourceLang, draft.targetLangs, rulesKey],
  );

  // The review analyses each target language against its memories, and
  // shows what pre-translation would fill.
  const review = useMemo(() => {
    if (step !== 4 || !c || !projectFiles) return null;
    const all = [...state.memories, ...created.map((n) => n.memory)];
    const out: Record<
      LangCode,
      {
        report: CoreReport;
        memories: string[];
        filled: number;
        confirmed: number;
      }
    > = {};
    for (const t of draft.targetLangs) {
      const refs = openRefs(
        c,
        memoryRefs(
          { id: "", memories: draft.memories, projectMemory: false },
          t,
          "lookup",
          all,
        ),
        all,
        state.projects,
      );
      const report = analyzeFiles(
        c,
        projectFiles,
        draft.sourceLang,
        t,
        refs,
        draft.context,
      );
      const pt =
        draft.prepare.pretranslate &&
        draft.prepare.minScore >= 50 &&
        draft.prepare.minScore <= 100
          ? pretranslateFiles(
              c,
              projectFiles,
              t,
              refs,
              draft.prepare,
              draft.context,
            )
          : { filled: 0, confirmed: 0 };
      out[t] = {
        report,
        memories: refs.map((r) => r.name),
        filled: pt.filled,
        confirmed: pt.confirmed,
      };
    }
    return out;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    step,
    c,
    projectFiles,
    JSON.stringify(draft.memories),
    draft.targetLangs,
    draft.sourceLang,
    draft.context,
    JSON.stringify(draft.prepare),
    created,
    state.memories,
  ]);
  const [reviewLang, setReviewLang] = useState<LangCode>("");
  const shownLang = draft.targetLangs.includes(reviewLang)
    ? reviewLang
    : (draft.targetLangs[0] ?? "");

  async function create() {
    if (!c || !projectFiles || problems.length) return;
    setCreating(true);
    const id = `p-${Date.now().toString(36)}`;
    const now = new Date().toISOString();
    const memoriesNow = [...state.memories, ...created.map((n) => n.memory)];
    let files = projectFiles;
    let filled = 0;
    if (draft.prepare.pretranslate) {
      for (const t of draft.targetLangs) {
        const refs = openRefs(
          c,
          memoryRefs(
            { id, memories: draft.memories, projectMemory: false },
            t,
            "lookup",
            memoriesNow,
          ),
          memoriesNow,
          state.projects,
        );
        const r = pretranslateFiles(
          c,
          files,
          t,
          refs,
          draft.prepare,
          draft.context,
        );
        files = r.files;
        filled += r.filled;
      }
    }
    const name = draft.name.trim();
    const addMemories: TranslationMemory[] = draft.projectMemory
      ? draft.targetLangs.map((t) => ({
          name: uniqueName(projectMemoryName(name, t), memoriesNow),
          sourceLang: draft.sourceLang,
          targetLang: t,
          client: draft.client.trim() || undefined,
          projectId: id,
          createdAt: now,
        }))
      : [];
    // New memories are saved first, so nothing the project refers to is missing.
    let kept = true;
    for (const n of created) {
      dispatch({ type: "createMemory", memory: n.memory });
      kept = (await persistMemory(c, n.memory.name)) && kept;
    }
    const t = draft.terminology;
    const pkgDoc = docs.find((d) => d.doc.sdlPackage)?.doc.sdlPackage;
    const project: Project = {
      id,
      name,
      client: draft.client.trim() || undefined,
      description: draft.description.trim() || undefined,
      sourceLang: draft.sourceLang,
      targetLangs: draft.targetLangs,
      files,
      memories: draft.memories,
      projectMemory: draft.projectMemory,
      setup: {
        mode: t.mode,
        masters: t.mode === "own" ? [] : t.masters,
        writeTo: t.mode === "direct" ? t.writeTo : undefined,
        location: t.location,
        autoUpdate: t.mode === "layered" && t.autoUpdate,
      },
      context: draft.context,
      qa: draft.qa,
      prepare: draft.prepare,
      segmentation: draft.segmentation.length ? draft.segmentation : undefined,
      createdAt: now,
      due: draft.due || undefined,
      progress: 0,
      sdlPackage: pkgDoc
        ? { bytes: pkgDoc.bytes, name: pkgDoc.name }
        : undefined,
    };
    if (where === "server" && remote.api && remote.conn) {
      setPublishProblem("");
      try {
        const done = await publish(c, remote.api, {
          project,
          draft: full,
          memories: memoriesNow,
          termbases: state.termbases,
          entries: state.entries,
          projects: state.projects,
        });
        if (asTemplate)
          dispatch({
            type: "saveTemplate",
            template: templateOf(`tpl-${Date.now().toString(36)}`, templateName, full, now),
          });
        notify(
          `Created project “${name}” on ${new URL(remote.conn.url).host}` +
            (done.uploaded.length ? `; uploaded ${done.uploaded.join(", ")}` : "") +
            (filled ? `; ${plural(filled, "segment", "segments")} pre-translated` : ""),
        );
        await remote.refresh();
        go({ page: "project", projectId: remoteId(done.project.id) });
      } catch (e) {
        setPublishProblem(errorText(e));
        setCreating(false);
      }
      return;
    }
    dispatch({
      type: "createProject",
      change: {
        project,
        addLanguages: t.addLanguages,
        addMemories,
        removeMemories: [],
      },
      now,
    });
    if (asTemplate)
      dispatch({
        type: "saveTemplate",
        template: templateOf(
          `tpl-${Date.now().toString(36)}`,
          templateName,
          full,
          now,
        ),
      });
    notify(
      `Created project “${name}”` +
        (filled
          ? `; ${plural(filled, "segment", "segments")} pre-translated`
          : "") +
        (asTemplate ? `; template “${templateName.trim()}” saved` : "") +
        (kept
          ? ""
          : ". The new memories are kept only until you close this page: this browser does not allow saving"),
    );
    const first = files.find(
      (f) => f.role === "translatable" && Object.keys(f.segments).length,
    );
    go({
      page: "editor",
      projectId: id,
      fileId: first?.id,
      lang: first ? (first.targetLang ?? draft.targetLangs[0]) : undefined,
    });
  }

  const canContinue = (i: number) =>
    stepProblems(i).length === 0 && (i !== 1 || translatable.length > 0);

  return (
    <div className="page page-narrow">
      <PageHeader title="New project">
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
            Start
          </h2>
          <fieldset className="field">
            <legend>Start from</legend>
            <ul className="choice-list">
              <li>
                <label className="choice">
                  <input
                    type="radio"
                    name="start"
                    checked={start.kind === "blank"}
                    onChange={() => applyStart({ kind: "blank" })}
                  />
                  <span className="choice-body">
                    <span className="choice-title">A blank project</span>
                    <span className="choice-desc">
                      Languages from the files, memories and termbases suggested
                      for the client.
                    </span>
                  </span>
                </label>
              </li>
              {state.templates.map((t) => (
                <li key={t.id}>
                  <label className="choice">
                    <input
                      type="radio"
                      name="start"
                      checked={start.kind === "template" && start.id === t.id}
                      onChange={() =>
                        applyStart({ kind: "template", id: t.id })
                      }
                    />
                    <span className="choice-body">
                      <span className="choice-title">
                        Template: {t.name}
                        {t.client && <span className="tag">{t.client}</span>}
                      </span>
                      <span className="choice-meta">
                        <span className="lang-pair">
                          <LangChip code={t.sourceLang} /> →{" "}
                          {t.targetLangs.map((l) => (
                            <LangChip key={l} code={l} />
                          ))}
                        </span>
                        <span>
                          {plural(
                            Object.values(t.memories).reduce(
                              (n, ms) => n + ms.length,
                              0,
                            ),
                            "memory",
                            "memories",
                          )}
                        </span>
                      </span>
                    </span>
                  </label>
                </li>
              ))}
              {state.projects.length > 0 && (
                <li>
                  <div className="choice">
                    <input
                      type="radio"
                      name="start"
                      id="start-project"
                      checked={start.kind === "project"}
                      onChange={() =>
                        applyStart({
                          kind: "project",
                          id: state.projects[0]!.id,
                        })
                      }
                    />
                    <span className="choice-body">
                      <label className="choice-title" htmlFor="start-project">
                        Like an earlier project
                      </label>
                      <span className="choice-desc">
                        Its languages, memories, termbases and checks; new files
                        and a new name.
                      </span>
                      {start.kind === "project" && (
                        <span className="choice-inline">
                          <label htmlFor="start-project-pick">Project</label>
                          <select
                            id="start-project-pick"
                            value={start.id}
                            onChange={(e) =>
                              applyStart({
                                kind: "project",
                                id: e.target.value,
                              })
                            }
                          >
                            {state.projects.map((p) => (
                              <option key={p.id} value={p.id}>
                                {p.name}
                                {p.client ? ` (${p.client})` : ""}
                              </option>
                            ))}
                          </select>
                        </span>
                      )}
                    </span>
                  </div>
                </li>
              )}
            </ul>
            {state.templates.length === 0 && (
              <p className="help">
                You can save any project's settings as a template, here at the
                end or later from the project's settings.
              </p>
            )}
          </fieldset>
          <div className="panel-actions">
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => setStep(1)}
            >
              Continue to files and languages
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
            Files and languages
          </h2>
          <FilesField docs={docs} setDocs={setDocs} />
          <Problems messages={problemsAt(problems, "files")} />

          {bilingualSources.length === 1 ? (
            <div className="field">
              <span className="field-label">Source language</span>
              <p className="lang-fixed">
                {lang(draft.sourceLang).name}{" "}
                <LangChip code={draft.sourceLang} />
              </p>
              <p className="help">
                From the bilingual files, which are made for it.
              </p>
            </div>
          ) : (
            <div className="field">
              <label htmlFor="source">Source language</label>
              <SourceDetection
                detection={detection}
                source={draft.sourceLang}
                touched={touched.source || start.kind !== "blank"}
                fallback={recentSource}
              />
              <select
                id="source"
                value={draft.sourceLang}
                onChange={(e) => {
                  touch("source");
                  const v = e.target.value;
                  setDraft((d) => ({
                    ...d,
                    sourceLang: v,
                    targetLangs: d.targetLangs.filter((t) => t !== v),
                  }));
                }}
                aria-describedby="source-help"
              >
                {LANGUAGES.map((l) => (
                  <option key={l.code} value={l.code}>
                    {l.name}
                  </option>
                ))}
                {draft.sourceLang &&
                  !LANGUAGES.some((l) => l.code === draft.sourceLang) && (
                    <option value={draft.sourceLang}>
                      {lang(draft.sourceLang).name}
                    </option>
                  )}
              </select>
              <p id="source-help" className="help">
                Text shows the language, not the region. Check the variant (for
                example German (Austria)) if the client needs it.
              </p>
              <Problems messages={problemsAt(problems, "sourceLang")} />
            </div>
          )}

          <fieldset className="field">
            <legend>Target languages</legend>
            <TargetPicker
              source={draft.sourceLang}
              targets={draft.targetLangs}
              setTargets={(ts) => {
                touch("targets");
                setDraft((d) => ({ ...d, targetLangs: ts }));
              }}
              suggestions={recentTargets}
              fixed={fixedTargets}
            />
            {draft.targetLangs.length > 1 && (
              <p className="help">
                Each file is translated into every target language; a bilingual
                file only into the language it was made for.
              </p>
            )}
            <Problems messages={problemsAt(problems, "targetLangs")} />
          </fieldset>

          <div className="field-grid">
            <div className="field">
              <label htmlFor="pname">Project name</label>
              <input
                id="pname"
                value={draft.name}
                onChange={(e) => {
                  touch("name");
                  setDraft((d) => ({ ...d, name: e.target.value }));
                }}
                aria-invalid={problemsAt(problems, "name").length > 0}
                aria-describedby="pname-err"
              />
              <Problems
                id="pname-err"
                messages={problemsAt(problems, "name")}
              />
            </div>
            <div className="field">
              <label htmlFor="client">
                Client <span className="optional">optional</span>
              </label>
              <input
                id="client"
                list="clients"
                value={draft.client}
                onChange={(e) =>
                  setDraft((d) => ({ ...d, client: e.target.value }))
                }
                aria-describedby="client-help"
              />
              <datalist id="clients">
                {clients.map((x) => (
                  <option key={x} value={x} />
                ))}
              </datalist>
              <p id="client-help" className="help">
                Used to suggest the client's memories and termbases.
              </p>
              <Problems messages={problemsAt(problems, "client")} />
            </div>
            <div className="field">
              <label htmlFor="due">
                Due <span className="optional">optional</span>
              </label>
              <input
                id="due"
                type="datetime-local"
                value={toLocalInput(draft.due)}
                onChange={(e) =>
                  setDraft((d) => ({
                    ...d,
                    due: fromLocalInput(e.target.value),
                  }))
                }
              />
              <Problems messages={problemsAt(problems, "due")} />
            </div>
            <div className="field">
              <label htmlFor="desc">
                Description <span className="optional">optional</span>
              </label>
              <input
                id="desc"
                value={draft.description}
                onChange={(e) =>
                  setDraft((d) => ({ ...d, description: e.target.value }))
                }
              />
            </div>
          </div>

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
              disabled={!c || !canContinue(1)}
              onClick={() => setStep(2)}
            >
              Continue to memories
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
            Memories
          </h2>
          <MemoriesSection
            c={c}
            draft={draft}
            setDraft={(f) => {
              touch("memories");
              setDraft(f);
            }}
            problems={problems}
            created={created}
            setCreated={setCreated}
          />
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
              disabled={!canContinue(2)}
              onClick={() => setStep(3)}
            >
              Continue to terminology
            </button>
          </div>
        </section>
      )}

      {step === 3 && (
        <section aria-labelledby="step-title" className="panel">
          <h2
            id="step-title"
            ref={stepHeading}
            tabIndex={-1}
            className="panel-title"
          >
            Terminology
          </h2>
          <TerminologySection
            draft={draft}
            setDraft={setDraft}
            problems={problems}
            onTouched={(k) => touch(k)}
          />
          <div className="panel-actions">
            <button
              type="button"
              className="btn btn-quiet"
              onClick={() => setStep(2)}
            >
              Back
            </button>
            <button
              type="button"
              className="btn btn-primary"
              disabled={!canContinue(3)}
              onClick={() => setStep(4)}
            >
              Continue to review
            </button>
          </div>
        </section>
      )}

      {step === 4 && (
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
              {draft.name.trim()}
              {draft.client.trim() && (
                <span className="muted"> for {draft.client.trim()}</span>
              )}
              {draft.due && (
                <span className="muted">, due {fmtDate(draft.due)}</span>
              )}
            </dd>
            <dt>Files</dt>
            <dd>
              {plural(translatable.length, "file", "files")} to translate
              {docs.length > translatable.length &&
                `, ${plural(docs.length - translatable.length, "reference file", "reference files")}`}
              {packages[0] && (
                <span className="muted">
                  {" "}
                  from the {packageKind(packages[0])} {packages[0]}
                </span>
              )}
            </dd>
            <dt>Languages</dt>
            <dd>
              {lang(draft.sourceLang).name} into{" "}
              {draft.targetLangs.map((t) => lang(t).name).join(", ")}
            </dd>
            <dt>Memories</dt>
            <dd>
              {draft.targetLangs.map((t) => (
                <div key={t}>
                  <LangChip code={t} />{" "}
                  {(draft.memories[t] ?? []).length
                    ? (draft.memories[t] ?? [])
                        .map(
                          (m) =>
                            `${m.memory}${m.penalty ? ` (−${m.penalty})` : ""}`,
                        )
                        .join(", ")
                    : "none"}
                  {draft.projectMemory && (
                    <span className="muted">, and a project memory</span>
                  )}
                </div>
              ))}
            </dd>
            <dt>Terminology</dt>
            <dd>
              <TerminologySummary draft={draft} />
            </dd>
          </dl>

          <details className="settings-details">
            <summary>
              Checks and context matches
              <span className="muted">
                {c &&
                  ` ${checksOn(c, draft.qa).on} of ${checksOn(c, draft.qa).of} checks on, context matches by ${draft.context === "both" ? "the segments before and after" : "the segment before"}`}
              </span>
            </summary>
            <ChecksSection
              c={c}
              targetLangs={draft.targetLangs}
              problems={problems}
              qa={draft.qa}
              setQa={(qa) => {
                touch("checks");
                setDraft((d) => ({ ...d, qa }));
              }}
              context={draft.context}
              setContext={(context) => setDraft((d) => ({ ...d, context }))}
            />
          </details>

          <details className="settings-details">
            <summary>
              Segmentation
              <span className="muted">
                {draft.segmentation.length ? ` ${plural(draft.segmentation.length, "rule", "rules")} of your own` : " as usual for the language"}
              </span>
            </summary>
            <SegmentationSection rules={draft.segmentation} setRules={(segmentation) => setDraft((d) => ({ ...d, segmentation }))} problems={problems} />
          </details>

          <PrepareSection
            prepare={draft.prepare}
            setPrepare={(prepare) => setDraft((d) => ({ ...d, prepare }))}
            problems={problems}
            summary={
              review && shownLang && review[shownLang]
                ? draft.targetLangs
                    .map(
                      (t) =>
                        `${lang(t).name}: ${plural(review[t]?.filled ?? 0, "segment", "segments")} filled, ${review[t]?.confirmed ?? 0} confirmed`,
                    )
                    .join(". ") + "."
                : undefined
            }
          />

          <section className="field" aria-labelledby="an-title">
            <h3 id="an-title" className="field-label">
              Match analysis
            </h3>
            {prepareError ? (
              <p className="error" role="alert">
                A document could not be read: {prepareError}
              </p>
            ) : !review || !projectFiles ? (
              <p className="muted" role="status">
                Analysing the files…
              </p>
            ) : (
              <>
                {draft.targetLangs.length > 1 && (
                  <div
                    className="tabs tabs-small"
                    role="tablist"
                    aria-label="Target language"
                  >
                    {draft.targetLangs.map((t) => (
                      <button
                        key={t}
                        type="button"
                        role="tab"
                        aria-selected={t === shownLang}
                        onClick={() => setReviewLang(t)}
                      >
                        {lang(t).name}
                      </button>
                    ))}
                  </div>
                )}
                {review[shownLang] && (
                  <>
                    <p className="help">
                      {review[shownLang]!.memories.length
                        ? `Against ${review[shownLang]!.memories.join(" and ")}, counted in this order: context matches first, then repetitions within a file and across files, then the best match.`
                        : "No memory for this language, so only repetitions count."}
                    </p>
                    <AnalysisTable
                      rows={review[shownLang]!.report.rows}
                      total={review[shownLang]!.report.total}
                      caption={`Match analysis, ${lang(shownLang).name}`}
                    />
                  </>
                )}
              </>
            )}
          </section>

          <div className="field">
            <label className="check">
              <input
                type="checkbox"
                checked={asTemplate}
                onChange={(e) => setAsTemplate(e.target.checked)}
              />
              <span>Save these settings as a template</span>
            </label>
            {asTemplate && (
              <div className="nested field">
                <label htmlFor="tpl-name">Template name</label>
                <input
                  id="tpl-name"
                  value={templateName}
                  onChange={(e) => setTemplateName(e.target.value)}
                />
                <p className="help">
                  Languages, memories, termbases, checks and pre-translation;
                  not the files, name or due date.
                </p>
                <Problems messages={problemsAt(problems, "template")} />
              </div>
            )}
          </div>

          {remote.conn && (
            <fieldset className="field">
              <legend className="field-label">Where the project is kept</legend>
              <label className="choice">
                <input type="radio" name="where" checked={where === "here"} onChange={() => setWhere("here")} />
                <span className="choice-body">
                  <span className="choice-title">On this computer</span>
                  <span className="choice-desc">Only you work on it, in this browser.</span>
                </span>
              </label>
              <label className="choice">
                <input type="radio" name="where" checked={where === "server"} onChange={() => setWhere("server")} />
                <span className="choice-body">
                  <span className="choice-title">On the team server {new URL(remote.conn.url).host}</span>
                  <span className="choice-desc">
                    Shared: you give work to translators and reviewers there, and
                    everyone's translations are saved on the server. Memories and
                    master termbases the server doesn't have yet are uploaded
                    with it; the project termbase is kept there.
                  </span>
                </span>
              </label>
            </fieldset>
          )}

          {publishProblem && (
            <div className="notice" role="alert">
              <AlertTriangle size={18} aria-hidden="true" />
              <div className="notice-body">
                <p className="notice-title">The project was not made on the server</p>
                <p>{publishProblem}</p>
              </div>
            </div>
          )}

          {problems.length > 0 && (
            <div className="notice" role="alert">
              <AlertTriangle size={18} aria-hidden="true" />
              <div className="notice-body">
                <p className="notice-title">The project can't be created yet</p>
                <ul>
                  {problems.map((p) => (
                    <li key={p.field + p.message}>{p.message}</li>
                  ))}
                </ul>
              </div>
            </div>
          )}

          <div className="panel-actions">
            <button
              type="button"
              className="btn btn-quiet"
              onClick={() => setStep(3)}
            >
              Back
            </button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={create}
              disabled={creating || !projectFiles || problems.length > 0}
            >
              {creating ? "Creating…" : where === "server" ? "Create on the server" : "Create project"}
            </button>
          </div>
        </section>
      )}
    </div>
  );
}

/** The project's files: add files or folders, choose what each is for, remove. */
export function FilesField({
  docs,
  setDocs,
  existing = [],
  label = "Files",
}: {
  /** Heading of the list. */
  label?: string;
  docs: DraftDoc[];
  setDocs: (f: (d: DraftDoc[]) => DraftDoc[]) => void;
  /** Names already in the project (on its settings page). */
  existing?: string[];
}) {
  const filesInput = useRef<HTMLInputElement>(null);
  const folderInput = useRef<HTMLInputElement>(null);
  const [reading, setReading] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [error, setError] = useState("");

  async function add(list: { file: File; path?: string }[]) {
    if (!list.length) return;
    setReading(true);
    setError("");
    try {
      const read = await readFiles(list);
      setDocs((d) => [...d, ...draftDocs(read)]);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setReading(false);
    }
  }

  async function onDrop(e: DragEvent) {
    e.preventDefault();
    setDragging(false);
    await add(await droppedFiles(e.dataTransfer));
  }

  const names = [...existing.map((n) => n.toLowerCase())];
  return (
    <div className="field">
      <span className="field-label" id="files-label">
        {label}
      </span>
      {docs.length > 0 && (
        <div className="table-wrap">
          <table className="table file-table" aria-labelledby="files-label">
            <thead>
              <tr>
                <th scope="col">File</th>
                <th scope="col" className="num">
                  Words
                </th>
                <th scope="col">Use</th>
                <th scope="col">
                  <span className="sr-only">Remove</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {docs.map((d) => {
                const name = d.doc.path ?? d.doc.file;
                const twice =
                  names.filter((n) => n === name.toLowerCase()).length > 0 ||
                  docs.filter(
                    (x) =>
                      (x.doc.path ?? x.doc.file).toLowerCase() ===
                      name.toLowerCase(),
                  ).length > 1;
                names.push(name.toLowerCase());
                const canTranslate = readable(d.doc);
                return (
                  <tr key={d.key}>
                    <th scope="row">
                      <span className="file-name">
                        <FileText size={15} aria-hidden="true" />
                        <span className="cell-title">{name}</span>
                      </span>
                      <span className="cell-sub">{describe(d.doc)}</span>
                      {d.doc.warnings?.map((w) => (
                        <span key={w} className="detect detect-warn">
                          <AlertTriangle size={14} aria-hidden="true" />
                          <span>{w}</span>
                        </span>
                      ))}
                      {!canTranslate && (
                        <span className="detect detect-warn">
                          <AlertTriangle size={14} aria-hidden="true" />
                          <span>
                            {whyNot(d.doc)} It is added as a reference file.
                          </span>
                        </span>
                      )}
                      {twice && (
                        <span className="error">
                          This file name is in the project twice.
                        </span>
                      )}
                    </th>
                    <td className="num">
                      {d.doc.words ? fmtNum(d.doc.words) : "–"}
                    </td>
                    <td>
                      <label className="sr-only" htmlFor={`role-${d.key}`}>
                        Use of {name}
                      </label>
                      <select
                        id={`role-${d.key}`}
                        className="select-small"
                        value={d.role}
                        disabled={!canTranslate}
                        onChange={(e) =>
                          setDocs((ds) =>
                            ds.map((x) =>
                              x.key === d.key
                                ? { ...x, role: e.target.value as FileRole }
                                : x,
                            ),
                          )
                        }
                      >
                        <option value="translatable">Translate</option>
                        <option value="reference">Reference</option>
                      </select>
                    </td>
                    <td>
                      <button
                        type="button"
                        className="icon-button"
                        aria-label={`Remove ${name}`}
                        onClick={() =>
                          setDocs((ds) => ds.filter((x) => x.key !== d.key))
                        }
                      >
                        <X size={14} aria-hidden="true" />
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      <div
        className={`dropzone dropzone-small${dragging ? " is-dragging" : ""}`}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => void onDrop(e)}
      >
        <Upload size={18} aria-hidden="true" className="dropzone-icon" />
        <p className="dropzone-text">
          {reading
            ? "Reading the files…"
            : "Drop files or folders here. DOCX, PPTX, XLSX, IDML and web page files keep their formatting; of a PDF the text is read, and its translation is a DOCX document; bilingual files (.sdlxliff, .mqxliff, .mqxlz, .mxliff, .txml, .ttx, .xlf), project packages (.sdlppx) and handoff packages (.mqout) keep their translations and states, and so do bilingual tables for review: Trados review documents, memoQ two-column RTF and DOCX, Déjà Vu RTF external views."}
        </p>
        <div className="row">
          <button
            type="button"
            className="btn btn-small"
            onClick={() => filesInput.current?.click()}
            disabled={reading}
          >
            <FileText size={14} aria-hidden="true" />
            Add files
          </button>
          <button
            type="button"
            className="btn btn-small"
            onClick={() => folderInput.current?.click()}
            disabled={reading}
          >
            <FolderOpen size={14} aria-hidden="true" />
            Add a folder
          </button>
        </div>
        <input
          ref={filesInput}
          type="file"
          multiple
          hidden
          onChange={(e) => {
            const list = e.target.files ? chosenFiles(e.target.files) : [];
            e.target.value = "";
            void add(list);
          }}
        />
        <input
          ref={folderInput}
          type="file"
          multiple
          hidden
          {...{ webkitdirectory: "" }}
          onChange={(e) => {
            const list = e.target.files ? chosenFiles(e.target.files) : [];
            e.target.value = "";
            void add(list);
          }}
        />
      </div>
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      <p className="help">
        <Info size={14} aria-hidden="true" /> Reference files (a style guide, an
        earlier version) travel with the project for the translators to read;
        they are not translated.
      </p>
    </div>
  );
}

function describe(doc: PendingDoc): string {
  if (doc.bilingual) {
    const bi = doc.bilingual;
    const done = bi.segments.filter((s) => s.status === "confirmed").length;
    const locked = bi.segments.filter((s) => s.locked).length;
    const what = doc.sdlPackage
      ? `From the ${packageKind(doc.sdlPackage.name)} ${doc.sdlPackage.name}`
      : (XLIFF_FORMAT_NAME[bi.format] ?? "XLIFF");
    const langs = bi.targetLang
      ? `, ${lang(knownLang(bi.sourceLang)).name} to ${lang(knownLang(bi.targetLang)).name}`
      : "";
    return `${what}${langs}, ${done} of ${plural(bi.segments.length, "segment", "segments")} confirmed${locked ? `, ${locked} locked` : ""}`;
  }
  switch (kindOf(doc)) {
    case "docx":
      return `${DOCUMENT_FORMAT_NAME[documentFormat(doc.file) ?? "docx"]}, ${DOCUMENT_FORMAT_KEEPS[documentFormat(doc.file) ?? "docx"]}`;
    default:
      return "No text Interlinea can read";
  }
}

function whyNot(doc: PendingDoc): string {
  if (doc.error) return `It could not be read: ${doc.error}`;
  return "Interlinea can't read text from this file (for example a scan, or a format it has no filter for). It can travel with the project for reference.";
}

function TerminologySummary({ draft }: { draft: Draft }) {
  const { state } = useLocalStore();
  const t = draft.terminology;
  const names = t.masters
    .map((id) => state.termbases.find((x) => x.id === id)?.name)
    .filter(Boolean)
    .join(", ");
  if (t.mode === "direct")
    return (
      <>
        New terms go straight into{" "}
        {state.termbases.find((x) => x.id === t.writeTo)?.name ?? names}.
      </>
    );
  if (t.mode === "own") return <>A new project termbase on this computer, no master.</>;
  return (
    <>
      A new project termbase on this computer, based on {names}.{" "}
      {t.autoUpdate
        ? "Takes in master changes each time the project opens."
        : "Master changes wait until you take them in."}
      {t.addLanguages.length > 0 &&
        ` Languages are added to ${t.addLanguages
          .map((id) => state.termbases.find((x) => x.id === id)?.name)
          .join(", ")}.`}
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
    const differs =
      detection.code &&
      detection.confidence >= CONFIDENT &&
      primary(detection.code) !== primary(source);
    return (
      <p className={`detect ${differs ? "detect-warn" : "detect-info"}`}>
        {differs ? (
          <AlertTriangle size={16} aria-hidden="true" />
        ) : (
          <Info size={16} aria-hidden="true" />
        )}
        <span>
          Set to {lang(source).name}.
          {differs &&
            ` The files look ${lang(detection.code!).name} (${Math.round(detection.confidence * 100)}%). Check the source language.`}
        </span>
      </p>
    );
  }
  if (detection.reason === "empty") {
    return (
      <p className="detect detect-warn">
        <AlertTriangle size={16} aria-hidden="true" />
        <span>
          No text found in the files (for example a scanned PDF), so the
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
        of the files.
      </span>
    </p>
  );
}

function uniqueName(name: string, taken: TranslationMemory[]): string {
  let out = name;
  for (
    let n = 2;
    taken.some((m) => m.name.toLowerCase() === out.toLowerCase());
    n++
  )
    out = `${name} ${n}`;
  return out;
}

/** An ISO date as the datetime-local input shows it, in local time. */
export function toLocalInput(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fromLocalInput(v: string): string {
  if (!v) return "";
  const d = new Date(v);
  return Number.isNaN(d.getTime())
    ? ""
    : d.toISOString().replace(/\.\d{3}Z$/, "Z");
}

function mostCommon<T>(xs: T[]): T | undefined {
  const m = new Map<T, number>();
  for (const x of xs) m.set(x, (m.get(x) ?? 0) + 1);
  return [...m.entries()].sort((a, b) => b[1] - a[1])[0]?.[0];
}
