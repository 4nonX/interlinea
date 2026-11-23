// Saved work from the first version of the prototype (state version 1),
// where a project had one document, one segment list for its first target
// language and an implicit memory, moved into the current shape. Nothing
// is dropped: the document becomes the project's file, its segments stay
// with the first target language, other target languages get fresh copies,
// and the project memory keeps the TMX imported into it.

import {
  DEFAULT_PREPARE,
  DEFAULT_QA,
  INITIAL_MEMORIES,
  SAMPLE_MEMORY,
  freshCopy,
  primary,
  projectMemoryName,
  projectProgress,
  type ContextRule,
  type Entry,
  type LangCode,
  type Location,
  type Project,
  type SetupMode,
  type Termbase,
  type TranslationMemory,
} from "./data";
import type { Block, Segment } from "./editor/model";
import type { SavedState } from "./store";

interface V1Project {
  id: string;
  name: string;
  file: string;
  words: number;
  sourceLang: LangCode;
  targetLangs: LangCode[];
  client?: string;
  setup: {
    mode: SetupMode;
    masters: string[];
    projectTermbaseId?: string;
    location: Location;
  };
  createdAt: string;
  due?: string;
  progress: number;
  segments?: Segment[];
  blocks?: Block[];
  original?: {
    url?: string;
    bytes?: Uint8Array;
    format?: "docx" | "xliff";
    sdlPackage?: { bytes: Uint8Array; name: string; path: string };
  };
}

export interface V1State {
  termbases: Termbase[];
  entries: Entry[];
  projects: V1Project[];
  baseSnapshots: Record<string, Entry>;
  branchedAt: Record<string, string>;
  gitWords: boolean;
  autoInsert: boolean;
  autoPropagate: boolean;
  contextRule: ContextRule;
}

export function migrateV1(old: V1State): {
  state: SavedState;
  /** Saved memories to move: [old key name, new name]. */
  renamedMemories: [string, string][];
} {
  const memories: TranslationMemory[] = INITIAL_MEMORIES.filter(
    (m) => !m.projectId,
  );
  const renamed: [string, string][] = [];
  const taken = new Set(memories.map((m) => m.name.toLowerCase()));
  const projects = old.projects.map((p): Project => {
    const first = p.targetLangs[0] ?? "en-GB";
    const bilingual = p.original?.format === "xliff";
    const segments: Record<LangCode, Segment[]> = {};
    if (p.segments) {
      segments[first] = p.segments;
      if (!bilingual)
        for (const t of p.targetLangs.slice(1))
          segments[t] = freshCopy(p.segments);
    }
    // The first version matched every project against the client's sample
    // memory when the client and languages fitted, and gave each project a
    // memory for its TMX imports and its confirmed segments.
    const sample =
      p.client === "Kanzlei Weber" &&
      primary(p.sourceLang) === "de" &&
      primary(first) === "en";
    let name = projectMemoryName(p.name, first);
    for (let n = 2; taken.has(name.toLowerCase()); n++)
      name = `${projectMemoryName(p.name, first)} ${n}`;
    taken.add(name.toLowerCase());
    memories.push({
      name,
      sourceLang: p.sourceLang,
      targetLang: first,
      client: p.client,
      projectId: p.id,
      createdAt: p.createdAt,
      seedFromProject: true,
    });
    renamed.push([`project:${p.id}`, name]);
    const project: Project = {
      id: p.id,
      name: p.name,
      client: p.client,
      sourceLang: p.sourceLang,
      targetLangs: p.targetLangs,
      files: [
        {
          id: `f-${p.id}`,
          name: p.file,
          role: "translatable",
          words: p.words,
          kind: p.original?.format ?? (p.segments ? "text" : "none"),
          targetLang: bilingual ? first : undefined,
          packagePath: p.original?.sdlPackage?.path,
          bytes: p.original?.bytes,
          url: p.original?.url,
          blocks: p.blocks,
          segments,
        },
      ],
      memories: sample
        ? {
            [first]: [
              {
                memory: SAMPLE_MEMORY,
                lookup: true,
                concordance: true,
                update: false,
                penalty: 0,
              },
            ],
          }
        : {},
      projectMemory: true,
      setup: {
        ...p.setup,
        writeTo: p.setup.mode === "direct" ? p.setup.masters[0] : undefined,
        autoUpdate: false,
      },
      context: old.contextRule ?? "previous",
      qa: DEFAULT_QA,
      prepare: DEFAULT_PREPARE,
      createdAt: p.createdAt,
      due: p.due,
      progress: p.progress,
      sdlPackage: p.original?.sdlPackage
        ? {
            bytes: p.original.sdlPackage.bytes,
            name: p.original.sdlPackage.name,
          }
        : undefined,
    };
    return { ...project, progress: projectProgress(project) };
  });
  return {
    state: {
      termbases: old.termbases,
      entries: old.entries,
      projects,
      memories,
      templates: [],
      dictionaries: [],
      baseSnapshots: old.baseSnapshots,
      branchedAt: old.branchedAt,
      gitWords: old.gitWords,
      autoInsert: old.autoInsert,
      autoPropagate: old.autoPropagate,
      contextRule: old.contextRule,
    },
    renamedMemories: renamed,
  };
}
