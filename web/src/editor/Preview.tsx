import { Fragment, useEffect, useRef, type ReactNode } from "react";
import type { Block, Run, Segment, TagRun } from "./model";
import { plain } from "./model";
import { tl } from "../i18n";

interface Props {
  blocks: Block[];
  segments: Segment[];
  activeId: string;
  sourceLang: string;
  targetLang: string;
  onPick: (id: string) => void;
}

/**
 * A live preview of the translated document, built from the paragraph
 * structure the document's filter reports: styles, tables, footnotes,
 * header and footer; for a workbook its sheets as tables, for a
 * presentation its slides' text in order. Segments without a translation show their source text. The saved
 * file comes from the original and is exact; this view is an approximation.
 */
export function Preview({
  blocks,
  segments,
  activeId,
  sourceLang,
  targetLang,
  onPick,
}: Props) {
  const root = useRef<HTMLDivElement>(null);
  const byBlock = new Map<string, Segment[]>();
  for (const s of segments) {
    if (!s.block) continue;
    byBlock.set(s.block, [...(byBlock.get(s.block) ?? []), s]);
  }
  // Footnote references are numbered in the order they appear in the body.
  const counter = { fn: 0 };

  useEffect(() => {
    root.current
      ?.querySelector(".pv-seg.is-active")
      ?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }, [activeId]);

  const renderBlock = (b: Block) => (
    <BlockView
      key={b.id}
      b={b}
      segs={byBlock.get(b.id) ?? []}
      activeId={activeId}
      sourceLang={sourceLang}
      targetLang={targetLang}
      onPick={onPick}
      counter={counter}
    />
  );

  const part = (name: Block["part"]) => blocks.filter((b) => b.part === name);
  const body = part("body");

  // Group consecutive table cells into tables.
  const bodyOut: ReactNode[] = [];
  for (let i = 0; i < body.length;) {
    const b = body[i]!;
    if (!b.cell) {
      bodyOut.push(renderBlock(b));
      i++;
      continue;
    }
    const table = b.cell.table;
    const cells: Block[] = [];
    while (i < body.length && body[i]!.cell?.table === table)
      cells.push(body[i++]!);
    const rows = new Map<number, Map<number, Block[]>>();
    for (const c of cells) {
      const row = rows.get(c.cell!.row) ?? new Map<number, Block[]>();
      row.set(c.cell!.col, [...(row.get(c.cell!.col) ?? []), c]);
      rows.set(c.cell!.row, row);
    }
    const cols = Math.max(...cells.map((c) => c.cell!.col)) + 1;
    bodyOut.push(
      <table key={`t${table}`} className="pv-table">
        <tbody>
          {[...rows.entries()].map(([r, row]) => (
            <tr key={r}>
              {Array.from({ length: cols }, (_, col) => (
                <td key={col}>{(row.get(col) ?? []).map(renderBlock)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>,
    );
  }

  const header = part("header");
  const footer = part("footer");
  const notes = [...part("footnote"), ...part("endnote")];
  const extras: { title: string; blocks: Block[] }[] = [
    { title: tl("Charts"), blocks: part("chart") },
    { title: "SmartArt", blocks: part("diagram") },
    { title: tl("Comments"), blocks: part("comment") },
    { title: tl("Speaker notes"), blocks: part("notes") },
    { title: tl("Title and description of the page"), blocks: part("meta") },
    { title: tl("Texts of pictures, links and form fields"), blocks: part("attribute") },
  ].filter((x) => x.blocks.length > 0);
  return (
    <div className="preview" ref={root} lang={targetLang}>
      <div className="pv-page">
        {header.length > 0 && (
          <div className="pv-header">{header.map(renderBlock)}</div>
        )}
        <div className="pv-body">{bodyOut}</div>
        {notes.length > 0 && (
          <div className="pv-notes">{notes.map(renderBlock)}</div>
        )}
        {footer.length > 0 && (
          <div className="pv-footer">{footer.map(renderBlock)}</div>
        )}
      </div>
      {extras.map((x) => (
        <div key={x.title} className="pv-extra">
          <p className="pv-extra-title">{x.title}</p>
          {x.blocks.map(renderBlock)}
        </div>
      ))}
    </div>
  );
}

function BlockView({
  b,
  segs,
  activeId,
  sourceLang,
  targetLang,
  onPick,
  counter,
}: {
  b: Block;
  segs: Segment[];
  activeId: string;
  sourceLang: string;
  targetLang: string;
  onPick: (id: string) => void;
  counter: { fn: number };
}) {
  const cls = ["pv-p"];
  if (b.frame === "textbox") cls.push("pv-frame");
  if (b.style === "label") cls.push("pv-label");
  if (b.heading)
    cls.push(
      b.style === "Title" ? "pv-title" : `pv-h${Math.min(b.heading, 3)}`,
    );
  if (b.list) cls.push("pv-list");
  if (b.align === "center") cls.push("pv-center");
  if (b.align === "right" || b.align === "end") cls.push("pv-right");
  if (b.align === "both") cls.push("pv-justify");
  const mark = (types: string[] | undefined, where: string) =>
    (types ?? []).map((t, i) =>
      t === "footnote" ? (
        <sup key={`${where}${i}`}>{++counter.fn}</sup>
      ) : t === "note-mark" ? (
        <sup key={`${where}${i}`}>{b.note}</sup>
      ) : t === "field" ? (
        <span key={`${where}${i}`} className="pv-field">
          #
        </span>
      ) : null,
    );
  return (
    <p className={cls.join(" ")}>
      {mark(b.lead, "l")}
      {segs.map((s, i) => {
        const translated = s.target.length > 0 && plain(s.target).trim() !== "";
        return (
          <Fragment key={s.id}>
            {i > 0 && " "}
            <span
              className={`pv-seg${s.id === activeId ? " is-active" : ""}${translated ? "" : " is-source"}`}
              lang={translated ? targetLang : sourceLang}
              onClick={() => onPick(s.id)}
              title={
                translated
                  ? undefined
                  : tl("Not translated yet: the source text is used")
              }
            >
              <Formatted
                runs={translated ? s.target : s.source}
                counter={counter}
              />
            </span>
          </Fragment>
        );
      })}
      {mark(b.trail, "t")}
    </p>
  );
}

/** Renders runs with the formatting their tags stand for. */
function Formatted({
  runs,
  counter,
}: {
  runs: Run[];
  counter: { fn: number };
}) {
  type Frame = { tag: TagRun | null; kids: ReactNode[] };
  const stack: Frame[] = [{ tag: null, kids: [] }];
  const wrap = (
    t: TagRun | null,
    kids: ReactNode[],
    key: number,
  ): ReactNode => {
    switch (t?.type) {
      case "bold":
        return <strong key={key}>{kids}</strong>;
      case "italic":
        return <em key={key}>{kids}</em>;
      case "underline":
        return <u key={key}>{kids}</u>;
      case "superscript":
        return <sup key={key}>{kids}</sup>;
      case "subscript":
        return <sub key={key}>{kids}</sub>;
      case "link":
        return (
          <span key={key} className="pv-link">
            {kids}
          </span>
        );
      default:
        return (
          <span key={key} className="pv-fmt">
            {kids}
          </span>
        );
    }
  };
  runs.forEach((r, i) => {
    const top = stack[stack.length - 1]!;
    if (typeof r === "string") {
      top.kids.push(r);
      return;
    }
    if (r.kind === "open") {
      stack.push({ tag: r, kids: [] });
      return;
    }
    if (r.kind === "close") {
      const at = stack.findLastIndex((f) => f.tag?.id === r.id);
      if (at <= 0) return;
      while (stack.length > at) {
        const f = stack.pop()!;
        stack[stack.length - 1]!.kids.push(
          wrap(f.tag, f.kids, i * 100 + stack.length),
        );
      }
      return;
    }
    switch (r.type) {
      case "footnote":
        top.kids.push(<sup key={i}>{++counter.fn}</sup>);
        break;
      case "lb":
        top.kids.push(<br key={i} />);
        break;
      case "tab":
        top.kids.push(<span key={i} className="pv-tab" />);
        break;
      case "field":
        top.kids.push(
          <span key={i} className="pv-field">
            #
          </span>,
        );
        break;
      case "image":
        top.kids.push(
          <span key={i} className="pv-image">
            {tl("image")}
          </span>,
        );
        break;
      default:
        break;
    }
  });
  while (stack.length > 1) {
    const f = stack.pop()!;
    stack[stack.length - 1]!.kids.push(
      wrap(f.tag, f.kids, 99999 + stack.length),
    );
  }
  return <>{stack[0]!.kids}</>;
}
