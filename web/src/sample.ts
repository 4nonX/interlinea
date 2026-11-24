// The sample project: a tenancy agreement of an invented law firm, with
// its master termbases, a project termbase with changes to merge and a
// translation memory. Every name and entry here is invented. It is added
// only when the person asks for it.

import {
  HOFMANN_BLOCKS,
  HOFMANN_FILE,
  HOFMANN_SEGMENTS,
  SAMPLE_TRANSLATOR,
  confirmedShare,
  wordCount,
} from "./editor/model";
import {
  DEFAULT_INFO_TYPES,
  DEFAULT_PREPARE,
  DEFAULT_QA,
  projectMemoryName,
  type Entry,
  type Project,
  type Term,
  type TermStatus,
  type Termbase,
  type TranslationMemory,
} from "./data";

const d = (s: string) => new Date(s).toISOString();

const TERMBASES: Termbase[] = [
  {
    id: "tb-weber",
    name: "Kanzlei Weber",
    kind: "master",
    location: "local",
    languages: ["de-DE", "en-GB"],
    client: "Kanzlei Weber",
    domain: "Tenancy law",
    infoTypes: DEFAULT_INFO_TYPES,
  },
  {
    id: "tb-mietrecht",
    name: "Mietrecht",
    kind: "master",
    location: "local",
    languages: ["de-DE", "en-GB", "fr-FR"],
    domain: "Tenancy law",
    infoTypes: DEFAULT_INFO_TYPES.filter((t) => t.id !== "grammar"),
  },
  {
    id: "tb-p-weber-lease",
    name: "Mietvertrag Hofmann (project)",
    kind: "project",
    location: "local",
    languages: ["de-DE", "en-GB"],
    client: "Kanzlei Weber",
    basedOn: ["tb-weber"],
    projectId: "p-hofmann",
    infoTypes: DEFAULT_INFO_TYPES,
  },
];

const t = (
  text: string,
  status: TermStatus = "approved",
  fields?: Record<string, string>,
): Term => ({ text, status, fields });

const ENTRIES: Entry[] = [
  // Master: Kanzlei Weber
  {
    id: "e1",
    termbaseId: "tb-weber",
    version: 3,
    concept: {
      domain: "Tenancy law",
      definition:
        "Die Sache, die der Vermieter dem Mieter zum Gebrauch überlässt.",
    },
    terms: {
      "de-DE": [t("Mietsache", "approved", { grammar: "noun, fem." })],
      "en-GB": [t("leased property"), t("rented property", "proposed")],
    },
    history: [
      {
        id: "c1",
        at: d("2025-03-04T09:12:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Created entry",
        project: "Gewerbemietvertrag Lang",
      },
      {
        id: "c2",
        at: d("2025-09-18T14:40:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Added English synonym “rented property” (proposed)",
        project: "Mietvertrag Schuster",
      },
      {
        id: "c3",
        at: d("2026-02-02T11:05:00Z"),
        by: "Eva Brandt",
        summary: "Added definition",
      },
    ],
  },
  {
    id: "e2",
    termbaseId: "tb-weber",
    version: 2,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [
        t("Kaution", "approved", { grammar: "noun, fem." }),
        t("Mietsicherheit"),
      ],
      "en-GB": [t("deposit"), t("security deposit")],
    },
    history: [
      {
        id: "c4",
        at: d("2025-03-04T09:20:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Created entry",
        project: "Gewerbemietvertrag Lang",
      },
      {
        id: "c5",
        at: d("2025-06-11T08:02:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Added “Mietsicherheit” and “security deposit”",
      },
    ],
  },
  {
    id: "e3",
    termbaseId: "tb-weber",
    version: 1,
    concept: {
      domain: "Tenancy law",
      definition:
        "Kosten, die dem Eigentümer durch das Eigentum am Grundstück laufend entstehen und auf den Mieter umgelegt werden dürfen.",
    },
    terms: {
      "de-DE": [t("Betriebskosten", "approved", { grammar: "noun, pl." })],
      "en-GB": [
        t("service charges"),
        t("operating costs", "forbidden", {
          note: "Client prefers “service charges” in residential leases.",
        }),
      ],
    },
    history: [
      {
        id: "c6",
        at: d("2025-04-22T16:30:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Created entry",
        project: "Mietvertrag Schuster",
      },
    ],
  },
  {
    id: "e4",
    termbaseId: "tb-weber",
    version: 2,
    concept: { domain: "Civil procedure" },
    terms: {
      "de-DE": [t("Räumungsklage", "approved", { grammar: "noun, fem." })],
      "en-GB": [t("action for possession")],
    },
    history: [
      {
        id: "c7",
        at: d("2025-05-02T10:00:00Z"),
        by: "Eva Brandt",
        summary: "Created entry",
      },
      // Changed on the master after the Hofmann project started: produces a conflict.
      {
        id: "c8",
        at: d("2026-11-18T15:22:00Z"),
        by: "Eva Brandt",
        summary: "Changed English term to “action for possession”",
      },
    ],
  },
  {
    id: "e5",
    termbaseId: "tb-weber",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [t("ordentliche Kündigung")],
      "en-GB": [t("termination with notice")],
    },
    history: [
      {
        id: "c9",
        at: d("2025-07-14T09:45:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Created entry",
      },
    ],
  },
  {
    id: "e6",
    termbaseId: "tb-weber",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [t("außerordentliche Kündigung")],
      "en-GB": [
        t("termination without notice"),
        t("extraordinary termination", "proposed"),
      ],
    },
    history: [
      {
        id: "c10",
        at: d("2025-07-14T09:47:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Created entry",
      },
    ],
  },
  {
    id: "e7",
    termbaseId: "tb-weber",
    version: 1,
    concept: { domain: "Contract law" },
    terms: {
      "de-DE": [t("Schriftformklausel")],
      "en-GB": [t("written form clause")],
    },
    history: [
      {
        id: "c11",
        at: d("2026-11-20T08:10:00Z"),
        by: "Eva Brandt",
        summary: "Created entry",
      },
    ],
  },
  {
    id: "e8",
    termbaseId: "tb-weber",
    version: 1,
    concept: { domain: "Contract law" },
    terms: {
      "de-DE": [t("salvatorische Klausel")],
      "en-GB": [t("severability clause")],
    },
    history: [
      {
        id: "c12",
        at: d("2026-11-20T08:14:00Z"),
        by: "Eva Brandt",
        summary: "Created entry",
      },
    ],
  },

  // Project termbase: Mietvertrag Hofmann (branch of Kanzlei Weber)
  {
    id: "p1",
    termbaseId: "tb-p-weber-lease",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [
        t("Schönheitsreparaturen", "proposed", { grammar: "noun, pl." }),
      ],
      "en-GB": [t("decorative repairs", "proposed")],
    },
    history: [
      {
        id: "pc1",
        at: d("2026-11-19T10:31:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Added while translating",
        project: "Mietvertrag Hofmann",
        segment:
          "Der Mieter übernimmt die Schönheitsreparaturen auf eigene Kosten.",
      },
    ],
  },
  {
    id: "p2",
    termbaseId: "tb-p-weber-lease",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [t("Staffelmiete", "proposed", { grammar: "noun, fem." })],
      "en-GB": [t("graduated rent", "proposed")],
    },
    history: [
      {
        id: "pc2",
        at: d("2026-11-19T11:02:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Added while translating",
        project: "Mietvertrag Hofmann",
        segment: "Die Parteien vereinbaren eine Staffelmiete gemäß § 557a BGB.",
      },
    ],
  },
  {
    id: "p3",
    termbaseId: "tb-p-weber-lease",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [t("Untervermietung", "proposed")],
      "en-GB": [t("subletting", "proposed")],
    },
    history: [
      {
        id: "pc3",
        at: d("2026-11-20T09:15:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Added while translating",
        project: "Mietvertrag Hofmann",
        segment: "Eine Untervermietung bedarf der Zustimmung des Vermieters.",
      },
    ],
  },
  {
    id: "p4",
    termbaseId: "tb-p-weber-lease",
    version: 1,
    concept: { domain: "Tenancy law" },
    terms: {
      "de-DE": [t("Mietminderung", "proposed")],
      "en-GB": [t("rent reduction", "proposed")],
    },
    history: [
      {
        id: "pc4",
        at: d("2026-11-20T14:48:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Added while translating",
        project: "Mietvertrag Hofmann",
        segment: "Das Recht zur Mietminderung bleibt unberührt.",
      },
    ],
  },
  {
    // Changed copy of master e2 (clean change: master unchanged since).
    id: "p5",
    termbaseId: "tb-p-weber-lease",
    version: 3,
    base: { termbaseId: "tb-weber", entryId: "e2", version: 2 },
    concept: {
      domain: "Tenancy law",
      definition:
        "Sicherheitsleistung des Mieters für Ansprüche des Vermieters aus dem Mietverhältnis.",
    },
    terms: {
      "de-DE": [
        t("Kaution", "approved", { grammar: "noun, fem." }),
        t("Mietsicherheit"),
      ],
      "en-GB": [
        t("deposit"),
        t("security deposit"),
        t("rental deposit", "proposed"),
      ],
    },
    history: [
      {
        id: "pc5",
        at: d("2026-11-19T10:50:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Added definition and “rental deposit” (proposed)",
        project: "Mietvertrag Hofmann",
        segment: "Die Kaution beträgt drei Nettokaltmieten.",
      },
    ],
  },
  {
    // Changed copy of master e4, but the master changed too: conflict.
    id: "p6",
    termbaseId: "tb-p-weber-lease",
    version: 2,
    base: { termbaseId: "tb-weber", entryId: "e4", version: 1 },
    concept: { domain: "Civil procedure" },
    terms: {
      "de-DE": [t("Räumungsklage", "approved", { grammar: "noun, fem." })],
      "en-GB": [t("eviction action")],
    },
    history: [
      {
        id: "pc6",
        at: d("2026-11-17T16:05:00Z"),
        by: SAMPLE_TRANSLATOR,
        summary: "Changed English term to “eviction action”",
        project: "Mietvertrag Hofmann",
        segment: "Der Vermieter kann Räumungsklage erheben.",
      },
    ],
  },
];

export const SAMPLE_MEMORY = "Kanzlei Weber (DE-EN)";

const MEMORIES: TranslationMemory[] = [
  {
    name: SAMPLE_MEMORY,
    sourceLang: "de-DE",
    targetLang: "en-GB",
    client: "Kanzlei Weber",
    createdAt: d("2024-03-12T09:00:00Z"),
    sample: true,
  },
  {
    name: projectMemoryName("Mietvertrag Hofmann", "en-GB"),
    sourceLang: "de-DE",
    targetLang: "en-GB",
    client: "Kanzlei Weber",
    projectId: "p-hofmann",
    createdAt: d("2026-11-16T08:30:00Z"),
    seedFromProject: true,
  },
];

const PROJECTS: Project[] = [
  {
    id: "p-hofmann",
    name: "Mietvertrag Hofmann",
    sourceLang: "de-DE",
    targetLangs: ["en-GB"],
    client: "Kanzlei Weber",
    files: [
      {
        id: "f-hofmann",
        name: "Mietvertrag_Hofmann_2026.docx",
        role: "translatable",
        words: HOFMANN_SEGMENTS.reduce((n, sg) => n + wordCount(sg.source), 0),
        kind: "docx",
        url: HOFMANN_FILE,
        blocks: HOFMANN_BLOCKS,
        segments: { "en-GB": HOFMANN_SEGMENTS },
      },
    ],
    memories: {
      "en-GB": [
        {
          memory: SAMPLE_MEMORY,
          lookup: true,
          concordance: true,
          update: false,
          penalty: 0,
        },
      ],
    },
    projectMemory: true,
    setup: {
      mode: "layered",
      masters: ["tb-weber"],
      projectTermbaseId: "tb-p-weber-lease",
      location: "local",
      autoUpdate: false,
    },
    context: "previous",
    qa: DEFAULT_QA,
    prepare: DEFAULT_PREPARE,
    createdAt: d("2026-11-16T08:30:00Z"),
    due: d("2026-11-26T16:00:00Z"),
    progress: confirmedShare(HOFMANN_SEGMENTS),
  },
];

/**
 * Master entries as they were when the Hofmann project copied them. The
 * project changed "eviction proceedings" to "eviction action" while the
 * master changed it to "action for possession": a conflict.
 */
const BASE_SNAPSHOTS: Record<string, Entry> = {
  p5: ENTRIES.find((e) => e.id === "e2")!,
  p6: {
    id: "e4",
    termbaseId: "tb-weber",
    version: 1,
    concept: { domain: "Civil procedure" },
    terms: {
      "de-DE": [t("Räumungsklage", "approved", { grammar: "noun, fem." })],
      "en-GB": [t("eviction proceedings")],
    },
    history: [
      {
        id: "c7",
        at: d("2025-05-02T10:00:00Z"),
        by: "Eva Brandt",
        summary: "Created entry",
      },
    ],
  },
};

/** The moment the Hofmann project was branched from its master. */
const BRANCHED_AT: Record<string, string> = {
  "p-hofmann": d("2026-11-16T08:30:00Z"),
};

/** Everything the sample adds: the project and what it works with. */
export const SAMPLE = {
  termbases: TERMBASES,
  entries: ENTRIES,
  projects: PROJECTS,
  memories: MEMORIES,
  baseSnapshots: BASE_SNAPSHOTS,
  branchedAt: BRANCHED_AT,
};
