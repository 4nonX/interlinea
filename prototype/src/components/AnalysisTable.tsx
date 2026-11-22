import { BANDS, type AnalysisRow } from "../editor/analysis";
import { fmtNum } from "../format";

/** The match analysis table: segments and words per band. */
export function AnalysisTable({
  rows,
  words,
  segments,
  caption,
}: {
  rows: AnalysisRow[];
  words: number;
  segments: number;
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
        {rows.map((r) => {
          const b = BANDS.find((x) => x.id === r.band)!;
          return (
            <tr key={r.band} className={r.words === 0 ? "is-zero" : undefined}>
              <th scope="row">
                <span className="analysis-band">
                  <span className={`score band-${r.band}`}>
                    {b.id === "cm"
                      ? "CM"
                      : b.id === "rep"
                        ? "Rep"
                        : b.id === "new"
                          ? "New"
                          : b.id === "100"
                            ? "100%"
                            : b.id === "50"
                              ? "50+"
                              : `${b.id}+`}
                  </span>
                  <span className="analysis-label">
                    {b.label}
                    {b.hint && <span className="analysis-hint">{b.hint}</span>}
                  </span>
                </span>
              </th>
              <td className="num">{fmtNum(r.segments)}</td>
              <td className="num">{fmtNum(r.words)}</td>
              <td className="num">
                {words ? `${Math.round((r.words / words) * 100)}%` : "0%"}
              </td>
            </tr>
          );
        })}
      </tbody>
      <tfoot>
        <tr>
          <th scope="row">Total</th>
          <td className="num">{fmtNum(segments)}</td>
          <td className="num">{fmtNum(words)}</td>
          <td className="num">100%</td>
        </tr>
      </tfoot>
    </table>
  );
}
