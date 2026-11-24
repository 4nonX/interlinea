import type { CoreBand, CoreCounts, CoreRow } from "../project";
import { fmtNum } from "../format";

const SHORT: Record<CoreBand, string> = {
  locked: "Lock",
  context: "CM",
  repetition: "Rep",
  "cross-file": "XRep",
  "100": "100%",
  "95": "95+",
  "85": "85+",
  "75": "75+",
  "50": "50+",
  new: "New",
};

const HINT: Record<CoreBand, string> = {
  locked: "Locked in the file; not translated",
  context: "100% match whose preceding segment also matches (101%)",
  repetition: "Same text and tags earlier in the same file",
  "cross-file": "Same text and tags in an earlier file of the project",
  "100": "Same text and tags as a memory unit",
  "95": "",
  "85": "",
  "75": "",
  "50": "Usually charged like new text",
  new: "No match of 50% or more",
};

const CLASS: Record<CoreBand, string> = {
  locked: "band-locked",
  context: "band-cm",
  repetition: "band-rep",
  "cross-file": "band-rep",
  "100": "band-100",
  "95": "band-95",
  "85": "band-85",
  "75": "band-75",
  "50": "band-50",
  new: "band-new",
};

/** The match analysis table from the core's report: segments and words per band. */
export function AnalysisTable({
  rows,
  total,
  caption,
}: {
  rows: CoreRow[];
  total: CoreCounts;
  caption: string;
}) {
  return (
    <table className="table analysis">
      <caption className="sr-only">{caption}</caption>
      <thead>
        <tr>
          <th scope="col">Match</th>
          <th scope="col" className="num">
            Segments
          </th>
          <th scope="col" className="num">
            Words
          </th>
          <th scope="col" className="num">
            Share
          </th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.band} className={r.words === 0 ? "is-zero" : undefined}>
            <th scope="row">
              <span className="analysis-band">
                <span className={`score ${CLASS[r.band]}`}>
                  {SHORT[r.band]}
                </span>
                <span className="analysis-label">
                  {r.name}
                  {HINT[r.band] && (
                    <span className="analysis-hint">{HINT[r.band]}</span>
                  )}
                </span>
              </span>
            </th>
            <td className="num">{fmtNum(r.segments)}</td>
            <td className="num">{fmtNum(r.words)}</td>
            <td className="num">
              {total.words
                ? `${Math.round((r.words / total.words) * 100)}%`
                : "0%"}
            </td>
          </tr>
        ))}
      </tbody>
      <tfoot>
        <tr>
          <th scope="row">Total</th>
          <td className="num">{fmtNum(total.segments)}</td>
          <td className="num">{fmtNum(total.words)}</td>
          <td className="num">100%</td>
        </tr>
      </tfoot>
    </table>
  );
}
