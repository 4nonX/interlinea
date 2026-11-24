import { useEffect, useMemo, useRef, useState } from "react";
import { FilePlus2, Trash2 } from "lucide-react";
import { DEFAULT_PREPARE, lang, type Termbase, type TranslationMemory } from "../data";
import { useStore } from "../store";
import { core, loadCore, tm, type CoreApi } from "../core";
import { errorText, qaOf, remoteId, useRemote } from "../remote";
import { ProjectAnalysis } from "./ProjectAnalysis";
import { ApiError, type CoreSettings, type ServerProject } from "../server";
import { coreSettings, type Draft, type Problem } from "../project";
import { readFiles } from "../detect";
import { buildFiles, draftDocs, extractWordDocs } from "../setup";
import { uploadOf } from "../publish";
import { plural } from "../format";
import {
  ChecksSection,
  MemoriesSection,
  PrepareSection,
  Problems,
  SectionsSourceProvider,
  TargetPicker,
  TerminologySection,
  type NewMemory,
  type SectionsSource,
} from "./ProjectSections";
import { ConfirmButton } from "./TermbaseDialogs";

/** A server project's settings as the sections edit them. */
function draftOf(s: CoreSettings): Draft {
  return {
    name: s.name,
    client: s.client ?? "",
    description: s.description ?? "",
    due: s.due ?? "",
    sourceLang: s.sourceLang,
    targetLangs: s.targetLangs,
    files: s.files.map((f) => ({ name: f.name, role: f.role, targetLang: f.targetLang || undefined })),
    memories: s.memories,
    projectMemory: s.projectMemory,
    terminology: {
      mode: s.terminology.mode,
      masters: s.terminology.masters,
      writeTo: s.terminology.writeTo || undefined,
      addLanguages: s.terminology.addLanguages ?? [],
      location: "server",
      autoUpdate: false,
    },
    context: s.analysis.context,
    qa: qaOf(s.qa),
    prepare: { ...DEFAULT_PREPARE, ...s.prepare },
  };
}

/**
 * The settings of a project on the team server, for its managers: target
 * languages, memories (the server's), terminology, checks and matches to
 * insert, saved together; and its files, added and removed.
 */
export function ServerProjectSettings({ project, onSaved }: { project: ServerProject; onSaved: () => Promise<void> }) {
  const { notify } = useStore();
  const r = useRemote();
  const api = r.api!;
  const [c, setC] = useState<CoreApi | null>(core());
  const [cat, setCat] = useState<Awaited<ReturnType<typeof api.catalog>> | null>(null);
  const [draft, setDraftState] = useState<Draft>(() => draftOf(project.settings));
  const [created, setCreated] = useState<NewMemory[]>([]);
  const [problems, setProblems] = useState<Problem[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [changed, setChangedState] = useState(false);
  const unsaved = useRef(false);
  const setChanged = (v: boolean) => {
    unsaved.current = v;
    setChangedState(v);
  };
  const setDraft = (f: (d: Draft) => Draft) => {
    setChanged(true);
    setDraftState(f);
  };

  useEffect(() => {
    if (!c) void loadCore().then(setC);
  }, [c]);
  useEffect(() => {
    void api.catalog().then(setCat, (e) => setError(errorText(e)));
  }, [api, project.version]);
  // The project changed on the server (a file, someone's save): start
  // again from it, unless changes here are not saved yet; saving those
  // then meets the newer version and says so.
  useEffect(() => {
    if (unsaved.current) return;
    setDraftState(draftOf(project.settings));
    setCreated([]);
    setProblems([]);
  }, [project.version, project.settings]);

  const source = useMemo<SectionsSource | null>(() => {
    if (!cat || !r.conn) return null;
    const host = new URL(r.conn.url).host;
    const memories: TranslationMemory[] = cat.memories.map((m) => ({
      name: m.name,
      sourceLang: m.sourceLang,
      targetLang: m.targetLang,
      createdAt: project.createdAt,
      remote: true,
    }));
    const termbases: Termbase[] = cat.termbases
      .filter((t) => t.name !== project.settings.terminology.termbase)
      .map((t) => ({
        id: t.name,
        name: t.name,
        kind: t.kind,
        location: "server",
        server: host,
        languages: t.languages,
        entryCount: t.entries,
        infoTypes: [],
        remote: true,
      }));
    return { memories, termbases, entries: [], units: Object.fromEntries(cat.memories.map((m) => [m.name, m.units])) };
  }, [cat, r.conn, project]);

  const fixed = project.settings.files.filter((f) => f.targetLang).map((f) => f.targetLang!);

  async function save() {
    setSaving(true);
    setError("");
    setProblems([]);
    const s = project.settings;
    try {
      // New memories go on the server first, with what was imported into them.
      for (const n of created) {
        await api.createMemory({ name: n.memory.name, sourceLang: n.memory.sourceLang, targetLang: n.memory.targetLang });
        if (n.units > 0 && c) await api.importTMX(n.memory.name, tm.exportTMX(c, n.memory.name));
      }
      const t = draft.terminology;
      let termbase = t.mode === "direct" ? "" : (s.terminology.termbase ?? "");
      // A project that now keeps its own terms gets its termbase; a
      // layered one follows the masters chosen.
      if (t.mode !== "direct" && !termbase) {
        termbase = `${draft.name} (project)`;
        if (t.mode === "layered") await api.createTermbase({ name: termbase }, t.masters);
        else await api.createTermbase({ name: termbase, languages: [draft.sourceLang, ...draft.targetLangs] });
      } else if (t.mode === "layered" && termbase && t.masters.join("\n") !== s.terminology.masters.join("\n")) {
        await api.setUpstreams(termbase, t.masters);
      }
      // Files are added and removed on their own; the settings keep the
      // project's current ones.
      const base = coreSettings({
        ...draft,
        files: s.files.map((f) => ({ name: f.name, role: f.role, targetLang: f.targetLang || undefined })),
      });
      const settings = {
        ...base,
        terminology: { ...base.terminology, termbase, location: "server", autoUpdate: false },
      } as CoreSettings;
      await api.updateSettings(project.id, settings, project.version);
      notify("Saved the project's settings on the server");
      setCreated([]);
      setChanged(false);
      setDraftState(draftOf(settings));
      await r.open(project.id).catch(() => undefined);
      await onSaved();
    } catch (e) {
      if (e instanceof ApiError && e.problems.length) setProblems(e.problems);
      else setError(e instanceof ApiError && e.code === "conflict" ? "The project changed on the server in the meantime. It is loaded again; make your change once more." : errorText(e));
      if (e instanceof ApiError && e.code === "conflict") {
        setChanged(false);
        await onSaved();
      }
    } finally {
      setSaving(false);
    }
  }

  if (!source)
    return (
      <p className="muted" role="status">
        Loading the server's memories and termbases…
      </p>
    );
  return (
    <SectionsSourceProvider value={source}>
      <ServerFiles project={project} onChanged={onSaved} />

      <section className="panel" aria-labelledby="sps-langs">
        <h2 id="sps-langs" className="panel-title">
          Languages and memories
        </h2>
        <TargetPicker
          source={draft.sourceLang}
          targets={draft.targetLangs}
          setTargets={(ts) => setDraft((d) => ({ ...d, targetLangs: ts }))}
          suggestions={project.settings.targetLangs}
          fixed={fixed}
        />
        <p className="help">
          A language you add gets every file to translate, with nothing translated yet. A language you take out loses its translations and the work given out in it.
        </p>
        <MemoriesSection c={c} draft={draft} setDraft={setDraft} problems={problems} created={created} setCreated={(f) => {
          setChanged(true);
          setCreated(f);
        }} projectId={remoteId(project.id)} />
      </section>

      <section className="panel" aria-labelledby="sps-terms">
        <h2 id="sps-terms" className="panel-title">
          Terminology
        </h2>
        <TerminologySection draft={draft} setDraft={setDraft} problems={problems} existing />
      </section>

      <section className="panel" aria-labelledby="sps-checks">
        <h2 id="sps-checks" className="panel-title">
          Checks and analysis
        </h2>
        <ChecksSection
          c={c}
          targetLangs={draft.targetLangs}
          problems={problems}
          qa={draft.qa}
          setQa={(qa) => setDraft((d) => ({ ...d, qa }))}
          context={draft.context}
          setContext={(context) => setDraft((d) => ({ ...d, context }))}
        />
        <ProjectAnalysis
          name={project.settings.name}
          run={async () =>
            (await r.api!.analysis(project.id)).map((rep) => ({
              lang: rep.targetLang,
              memories: (project.settings.memories[rep.targetLang] ?? []).filter((m) => m.lookup).map((m) => m.memory),
              report: rep,
            }))
          }
        />
      </section>

      <section className="panel" aria-labelledby="sps-prep">
        <h2 id="sps-prep" className="panel-title">
          Matches to insert
        </h2>
        <PrepareSection prepare={draft.prepare} setPrepare={(prepare) => setDraft((d) => ({ ...d, prepare }))} problems={problems} existing />
        <button
          type="button"
          className="btn"
          disabled={changed}
          title={changed ? "Save the settings first" : undefined}
          onClick={() =>
            void api.pretranslate(project.id).then(
              (rs) => {
                const n = rs.reduce((a, x) => a + x.filled, 0);
                notify(n ? `Pre-translated ${plural(n, "segment", "segments")} on the server` : "Nothing to pre-translate: no empty segment has a good enough match");
              },
              (e) => notify(errorText(e)),
            )
          }
        >
          Pre-translate empty segments now
        </button>
      </section>

      <div className="page-actions save-bar">
        {problems.length > 0 && <Problems messages={problems.map((p) => p.message)} />}
        {error && (
          <p className="error" role="alert">
            Could not save: {error}
          </p>
        )}
        <button type="button" className="btn btn-primary" disabled={!changed || saving} onClick={() => void save()}>
          {saving ? "Saving…" : "Save settings"}
        </button>
      </div>
    </SectionsSourceProvider>
  );
}

/** A server project's files: added (read here, sent with their segments) and removed. */
function ServerFiles({ project, onChanged }: { project: ServerProject; onChanged: () => Promise<void> }) {
  const { notify } = useStore();
  const r = useRemote();
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState("");
  const s = project.settings;

  async function add(list: FileList) {
    setBusy("Reading the files…");
    try {
      const docs = draftDocs(await readFiles([...list].map((file) => ({ file }))));
      const words = await extractWordDocs(docs, s.sourceLang);
      const files = buildFiles(docs, words, s.targetLangs, (i) => `new${i}`) ?? [];
      let version = project.version;
      for (const f of files) {
        if (s.files.some((x) => x.name.toLowerCase() === f.name.toLowerCase())) {
          notify(`${f.name} is in the project already; remove it first to replace it`);
          continue;
        }
        setBusy(`Sending ${f.name}…`);
        version = (await r.api!.addFile(project.id, uploadOf(f), version)).version;
      }
      notify(`Added ${plural(files.length, "file", "files")} on the server`);
      await r.open(project.id).catch(() => undefined);
      await onChanged();
    } catch (e) {
      notify(errorText(e));
      await onChanged();
    } finally {
      setBusy("");
      if (input.current) input.current.value = "";
    }
  }

  return (
    <section className="panel" aria-labelledby="sps-files">
      <h2 id="sps-files" className="panel-title">
        Files
      </h2>
      <ul className="plain-list">
        {s.files.map((f) => (
          <li key={f.name} className="server-file">
            <span className="cell-title">{f.name}</span>
            <span className="cell-sub">
              {f.role === "reference"
                ? "Reference file"
                : f.targetLang
                  ? `Translated into ${lang(f.targetLang).name} only`
                  : `Translated into ${s.targetLangs.map((t) => lang(t).name).join(", ")}`}
            </span>
            <ConfirmButton
              label={
                <>
                  <Trash2 size={14} aria-hidden="true" /> Remove
                </>
              }
              confirm={`Remove ${f.name} with its translations and the work given out in it, for everyone?`}
              action="Remove"
              onConfirm={() =>
                void r.api!.removeFile(project.id, f.name, project.version).then(
                  async () => {
                    notify(`Removed ${f.name} on the server`);
                    await r.open(project.id).catch(() => undefined);
                    await onChanged();
                  },
                  (e) => notify(errorText(e)),
                )
              }
            />
          </li>
        ))}
      </ul>
      <input
        ref={input}
        type="file"
        multiple
        hidden
        aria-label="Files to add"
        onChange={(e) => e.target.files?.length && void add(e.target.files)}
      />
      <button type="button" className="btn" disabled={!!busy} onClick={() => input.current?.click()}>
        <FilePlus2 size={15} aria-hidden="true" /> {busy || "Add files"}
      </button>
    </section>
  );
}
