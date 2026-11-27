// The match analysis of a project's files, on its settings page: counted
// when asked for, against the saved settings, one target language at a
// time, and saved as CSV for a quote.

import { useState } from "react";
import { BarChart3, Download } from "lucide-react";
import { lang, type LangCode } from "../data";
import { download } from "../core";
import type { CoreReport } from "../project";
import { AnalysisTable } from "../components/AnalysisTable";
import { tl, tlCore } from "../i18n";
import { listOf } from "../format";

export interface LanguageAnalysis {
  lang: LangCode;
  /** The memories looked up in, in the project's order. */
  memories: string[];
  report: CoreReport;
}

const quote = (s: string) => `"${s.replace(/"/g, '""')}"`;

/** The analysis as CSV: one line for each band of each file and language. */
export function analysisCsv(all: LanguageAnalysis[]): string {
  const lines = [[tl("Language"), tl("File"), tl("Match"), tl("Segments"), tl("Words"), tl("Characters")].map(quote).join(",")];
  for (const a of all)
    for (const f of a.report.files)
      for (const r of f.rows)
        lines.push([quote(a.lang), quote(f.name), quote(tlCore(r.name)), r.segments, r.words, r.characters].join(","));
  return lines.join("\r\n") + "\r\n";
}

export function ProjectAnalysis({
  name,
  run,
}: {
  /** The project's name, for the file the analysis is saved as. */
  name: string;
  run: () => Promise<LanguageAnalysis[]>;
}) {
  const [all, setAll] = useState<LanguageAnalysis[]>([]);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [shown, setShown] = useState<LangCode>("");
  const current = all.find((a) => a.lang === shown) ?? all[0];
  const analyse = async () => {
    setBusy(true);
    setProblem("");
    try {
      setAll(await run());
    } catch (e) {
      setProblem(e instanceof Error ? e.message : String(e));
    }
    setBusy(false);
  };
  return (
    <section className="field" aria-labelledby="project-analysis">
      <span id="project-analysis" className="field-label">
        {tl("Match analysis")}
      </span>
      <div className="row">
        <button type="button" className="btn btn-small" disabled={busy} onClick={analyse}>
          <BarChart3 size={14} aria-hidden="true" /> {busy ? tl("Analysing…") : all.length ? tl("Analyse again") : tl("Analyse the files")}
        </button>
        {all.length > 0 && (
          <button
            type="button"
            className="btn btn-quiet btn-small"
            onClick={() => download(new TextEncoder().encode("﻿" + analysisCsv(all)), tl("{name} analysis.csv", { name }), "text/csv")}
          >
            <Download size={14} aria-hidden="true" />{" "}{tl("Save as CSV")}
          </button>
        )}
      </div>
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}
      {!current && <p className="help">{tl("Counts the words to translate by how well the project's memories match them, with the saved settings.")}</p>}
      {all.length > 1 && (
        <div className="tabs tabs-small" role="tablist" aria-label={tl("Target language")}>
          {all.map((a) => (
            <button key={a.lang} type="button" role="tab" aria-selected={a.lang === current?.lang} onClick={() => setShown(a.lang)}>
              {lang(a.lang).name}
            </button>
          ))}
        </div>
      )}
      {current && (
        <>
          <p className="help">
            {current.memories.length
              ? tl("Against {join}, counted in this order: context matches first, then repetitions within a file and across files, then the best match.", { join: listOf(current.memories) })
              : tl("No memory for this language, so only repetitions count.")}
          </p>
          <AnalysisTable rows={current.report.rows} total={current.report.total} caption={tl("Match analysis, {name}", { name: lang(current.lang).name })} />
        </>
      )}
    </section>
  );
}
