import {
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  type KeyboardEvent,
  type Ref,
} from "react";
import { normalize, type Run, type TagRun } from "./model";

export interface TargetEditorHandle {
  focus(): void;
  setRuns(runs: Run[]): void;
  insertText(text: string): void;
  insertTag(tag: TagRun): void;
  selectedText(): string;
}

export function tagDescription(t: TagRun): string {
  const what =
    t.desc ??
    (t.label === "b"
      ? "bold"
      : t.label === "fn"
        ? "footnote"
        : t.label === "br"
          ? "line break"
          : t.label);
  return t.kind === "open"
    ? `Tag ${t.id} start, ${what}`
    : t.kind === "close"
      ? `Tag ${t.id} end, ${what}`
      : `Tag ${t.id}, ${what}`;
}

export function chipText(t: TagRun): string {
  return t.kind === "open"
    ? `${t.id}›`
    : t.kind === "close"
      ? `‹${t.id}`
      : `${t.id}`;
}

function chip(t: TagRun): HTMLElement {
  const el = document.createElement("span");
  el.className = `tag-chip tag-${t.kind}`;
  el.contentEditable = "false";
  el.dataset.kind = t.kind;
  el.dataset.id = t.id;
  el.dataset.label = t.label;
  el.setAttribute("role", "img");
  el.setAttribute("aria-label", tagDescription(t));
  if (t.desc) el.title = t.desc;
  if (t.type) el.dataset.type = t.type;
  if (t.desc) el.dataset.desc = t.desc;
  el.textContent = chipText(t);
  return el;
}

function render(el: HTMLElement, runs: Run[]) {
  el.textContent = "";
  for (const r of runs)
    el.append(typeof r === "string" ? document.createTextNode(r) : chip(r));
}

function read(el: HTMLElement): Run[] {
  const out: Run[] = [];
  const walk = (n: Node) => {
    if (n.nodeType === Node.TEXT_NODE) {
      out.push(n.textContent ?? "");
      return;
    }
    if (!(n instanceof HTMLElement)) return;
    if (n.dataset.kind) {
      out.push({
        kind: n.dataset.kind as TagRun["kind"],
        id: n.dataset.id!,
        label: n.dataset.label ?? "",
        type: n.dataset.type,
        desc: n.dataset.desc,
      });
      return;
    }
    if (n.tagName === "BR") return;
    n.childNodes.forEach(walk);
  };
  el.childNodes.forEach(walk);
  // Browsers insert non-breaking spaces at the edges of editable text.
  return normalize(
    out.map((r) => (typeof r === "string" ? r.replace(/ /g, " ") : r)),
  );
}

function caretInside(el: HTMLElement): Range | null {
  const sel = window.getSelection();
  if (!sel || !sel.rangeCount) return null;
  const range = sel.getRangeAt(0);
  return el.contains(range.commonAncestorContainer) ? range : null;
}

function caretToEnd(el: HTMLElement) {
  const range = document.createRange();
  range.selectNodeContents(el);
  range.collapse(false);
  const sel = window.getSelection();
  sel?.removeAllRanges();
  sel?.addRange(range);
}

interface Props {
  initial: Run[];
  lang: string;
  label: string;
  describedBy?: string;
  onChange: (runs: Run[]) => void;
  onKeyDown: (e: KeyboardEvent<HTMLDivElement>) => void;
  ref?: Ref<TargetEditorHandle>;
}

/**
 * The target text of the active segment. The DOM is the source of truth while
 * typing (so input methods and the caret are never disturbed); React only
 * reads it back. Tags are atomic chips that cannot be split or edited.
 */
export function TargetEditor({
  initial,
  lang,
  label,
  describedBy,
  onChange,
  onKeyDown,
  ref,
}: Props) {
  const el = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    if (!el.current) return;
    render(el.current, initial);
    el.current.focus();
    caretToEnd(el.current);
    // Rendered once per segment; the component is keyed by segment id.
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useImperativeHandle(ref, () => ({
    focus() {
      el.current?.focus();
    },
    setRuns(runs) {
      if (!el.current) return;
      render(el.current, runs);
      el.current.focus();
      caretToEnd(el.current);
      onChange(read(el.current));
    },
    insertText(text) {
      const node = el.current;
      if (!node) return;
      node.focus();
      if (!caretInside(node)) caretToEnd(node);
      // execCommand keeps the browser's undo history; fall back to a range insert.
      if (!document.execCommand("insertText", false, text)) {
        const range = caretInside(node)!;
        range.deleteContents();
        const t = document.createTextNode(text);
        range.insertNode(t);
        range.setStartAfter(t);
        range.collapse(true);
      }
      onChange(read(node));
    },
    insertTag(tag) {
      const node = el.current;
      if (!node) return;
      node.focus();
      let range = caretInside(node);
      if (!range) {
        caretToEnd(node);
        range = caretInside(node)!;
      }
      range.deleteContents();
      const c = chip(tag);
      range.insertNode(c);
      range.setStartAfter(c);
      range.collapse(true);
      const sel = window.getSelection();
      sel?.removeAllRanges();
      sel?.addRange(range);
      onChange(read(node));
    },
    selectedText() {
      const node = el.current;
      if (!node) return "";
      const range = caretInside(node);
      return range ? range.toString().trim() : "";
    },
  }));

  return (
    <div
      ref={el}
      className="target-editor"
      role="textbox"
      aria-multiline="true"
      aria-label={label}
      aria-describedby={describedBy}
      contentEditable
      suppressContentEditableWarning
      spellCheck
      lang={lang}
      onInput={() => el.current && onChange(read(el.current))}
      onKeyDown={onKeyDown}
    />
  );
}
