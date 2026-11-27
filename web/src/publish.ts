// Making a project on the team server from a project set up here: the
// files with their segments go up with the settings; memories and master
// termbases the server doesn't have yet go up first, with their units and
// entries, so the project finds them there.

import { documentFormat, tm, type CoreApi } from "./core";
import type { Entry, Project, ProjectFile, Termbase, TranslationMemory } from "./data";
import { projectMemoryName } from "./data";
import { openMemory } from "./editor/memory";
import { coreSettings, type Draft } from "./project";
import { runsToCore } from "./editor/model";
import { term, toCoreTermbase } from "./termio";
import { ApiError, base64, type Client, type CoreSettings, type FileUpload, type ServerProject } from "./server";
import { tl } from "./i18n";

export interface PublishInput {
  project: Project;
  draft: Draft;
  memories: TranslationMemory[];
  termbases: Termbase[];
  entries: Entry[];
  projects: Project[];
}

/** What publishing did besides making the project. */
export interface Published {
  project: ServerProject;
  /** Memories and termbases made on the server for it. */
  uploaded: string[];
}

const fold = (s: string) => s.trim().toLowerCase();

export async function publish(c: CoreApi, api: Client, x: PublishInput): Promise<Published> {
  const { project: p, draft } = x;
  const cat = await api.catalog();
  const uploaded: string[] = [];
  const onServer = new Set(cat.memories.map((m) => fold(m.name)));

  // Memories the project uses that only this computer has.
  for (const name of new Set(Object.values(p.memories).flat().map((m) => m.memory))) {
    if (onServer.has(fold(name))) continue;
    const m = x.memories.find((y) => y.name === name);
    if (!m) throw new Error(tl("The memory {name} is not on this computer or the server.", { name }));
    try {
      await api.createMemory({ name: m.name, sourceLang: m.sourceLang, targetLang: m.targetLang });
    } catch (e) {
      if (e instanceof ApiError && e.status === 403)
        throw new Error(tl("{name} is only on this computer, and you may not make memories on the server. Ask its administrator to upload it, or take it out of the project.", { name }));
      throw e;
    }
    openMemory(c, m, x.projects);
    if (tm.count(c, m.name) > 0) await api.importMemory(m.name, tm.exportTMX(c, m.name));
    onServer.add(fold(name));
    uploaded.push(tl("memory {name}", { name }));
  }

  // Masters the project uses that the server doesn't have, by name.
  const tbOnServer = new Set(cat.termbases.map((t) => fold(t.name)));
  const t = draft.terminology;
  const masterIds = t.mode === "own" ? [] : t.masters;
  const masterNames: Record<string, string> = {};
  for (const id of masterIds) {
    const tb = x.termbases.find((y) => y.id === id);
    if (!tb) throw new Error(tl("A master termbase of the project is missing."));
    masterNames[id] = tb.name;
    if (tbOnServer.has(fold(tb.name))) continue;
    const full = toCoreTermbase(tb, x.entries.filter((e) => e.termbaseId === id));
    try {
      await api.createTermbase({ name: tb.name, languages: full.languages ?? [], fields: full.fields ?? [] });
    } catch (e) {
      if (e instanceof ApiError && e.status === 403)
        throw new Error(tl("The termbase {name} is not on the server, and you may not make termbases there. Ask its administrator to upload it, or choose other terminology.", { name: tb.name }));
      throw e;
    }
    if ((full.entries ?? []).length) await api.importTermbase(tb.name, term.write(c, full, "tbx"));
    tbOnServer.add(fold(tb.name));
    uploaded.push(tl("termbase {name}", { name: tb.name }));
  }

  // The project's own memories are made by the server with the project,
  // first in each language's list so they win ties.
  const newMemories = draft.projectMemory
    ? p.targetLangs.map((l) => ({ Name: projectMemoryName(p.name, l), SourceLang: p.sourceLang, TargetLang: l }))
    : [];
  for (const m of newMemories)
    if (onServer.has(fold(m.Name)))
      throw new Error(tl("A memory called {Name} is on the server already. Rename the project, or keep it without its own memory.", { Name: m.Name }));
  const memories: CoreSettings["memories"] = {};
  for (const l of p.targetLangs) {
    memories[l] = [
      ...(draft.projectMemory
        ? [{ memory: projectMemoryName(p.name, l), lookup: true, concordance: true, update: true, penalty: 0 }]
        : []),
      ...(p.memories[l] ?? []),
    ];
  }

  const base = coreSettings({ ...draft, name: p.name, files: p.files.map((f) => ({ name: f.name, role: f.role, targetLang: f.targetLang })) });
  const settings = {
    ...base,
    memories,
    terminology: {
      ...base.terminology,
      masters: base.terminology.masters.map((id) => masterNames[id] ?? id),
      writeTo: base.terminology.writeTo ? (masterNames[base.terminology.writeTo] ?? base.terminology.writeTo) : "",
      addLanguages: (base.terminology.addLanguages ?? []).map((id) => masterNames[id] ?? id),
      location: "server",
      autoUpdate: false,
    },
  } as CoreSettings;
  // The server spells with its own dictionaries: a language whose
  // dictionary is only in this browser is not spell-checked there.
  const serverDicts = new Set((await api.dictionaries()).map((d) => fold(d.name)));
  const qa = (settings as { qa?: { dictionaries?: Record<string, string>; grammar?: string } }).qa;
  // And it checks grammar with its own services: one that is only in
  // this browser is not asked there.
  if (qa?.grammar && !(await api.providers()).some((x) => x.kind === "check" && fold(x.name) === fold(qa.grammar!))) delete qa.grammar;
  if (qa?.dictionaries)
    qa.dictionaries = Object.fromEntries(Object.entries(qa.dictionaries).filter(([, n]) => serverDicts.has(fold(n))));

  const files: FileUpload[] = p.files.map(uploadOf);

  const made = await api.createProject({
    settings,
    files,
    memories: newMemories,
    termbase: t.mode !== "direct" ? { name: tl("{name} (project)", { name: p.name }) } : undefined,
  });
  return { project: made, uploaded };
}

/** A project file as the server takes it: its bytes and its segments. */
export function uploadOf(f: ProjectFile): FileUpload {
  const bytes =
    f.bytes ??
    new TextEncoder().encode(
      Object.values(f.segments)[0]?.map((s) => s.source.filter((r) => typeof r === "string").join("")).join("\n\n") ?? "",
    );
  return {
    name: f.name,
    role: f.role,
    targetLang: f.targetLang,
    format: f.role === "reference" ? (/\.([^.]+)$/.exec(f.name)?.[1]?.toLowerCase() ?? "") : f.kind === "text" ? "txt" : f.kind === "docx" ? (documentFormat(f.name) ?? "docx") : /\.mqxlz$/i.test(f.name) ? "mqxlz" : f.kind,
    data: base64(bytes),
    segments:
      f.role === "translatable"
        ? Object.fromEntries(
            Object.entries(f.segments).map(([l, segs]) => [
              l,
              segs.map((s) => ({
                id: s.id,
                source: runsToCore(s.source),
                target: runsToCore(s.target),
                status: s.status,
                locked: s.locked,
              })),
            ]),
          )
        : undefined,
  };
}
