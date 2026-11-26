import {
  useId,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type Ref,
} from "react";
import { normalize, type Run, type TagRun } from "./model";
import { tl } from "../i18n";

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
          ? tl("line break")
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

export function render(el: HTMLElement, runs: Run[]) {
  el.textContent = "";
  for (const r of runs)
    el.append(typeof r === "string" ? document.createTextNode(r) : chip(r));
}

/**
 * The runs an element shows. Text a person typed has the non-breaking
 * spaces browsers put at its edges turned back into spaces; asIs leaves
 * text that was only shown as it is.
 */
export function read(el: HTMLElement, asIs = false): Run[] {
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
  if (asIs) return normalize(out);
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

/** The letters and digits before a place in a text that are the start of a word being typed. */
const WORD_START = /[\p{L}\p{N}][\p{L}\p{N}\p{M}'’-]*$/u;
const WORD_GOES_ON = /^[\p{L}\p{N}\p{M}]/u;

/**
 * What can complete the word being typed: the candidates that start with
 * it, whatever its case, the shortest first.
 */
export function completionsFor(prefix: string, candidates: string[], limit = 5): string[] {
  const p = prefix.toLocaleLowerCase();
  if ([...p].length < 2) return [];
  const seen = new Set<string>();
  return candidates
    .filter((c) => {
      const k = c.toLocaleLowerCase();
      if (k === p || !k.startsWith(p) || seen.has(k)) return false;
      seen.add(k);
      return true;
    })
    .sort((a, b) => a.length - b.length || a.localeCompare(b))
    .slice(0, limit);
}

/** Completions offered at the caret. */
interface Offer {
  prefix: string;
  items: string[];
  index: number;
  left: number;
  top: number;
}

interface Props {
  initial: Run[];
  lang: string;
  label: string;
  describedBy?: string;
  onChange: (runs: Run[]) => void;
  onKeyDown: (e: KeyboardEvent<HTMLDivElement>) => void;
  /**
   * Words and terms that may complete what is being typed: the terms found
   * for the segment and the words of its memory matches.
   */
  completions?: string[];
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
  completions,
  ref,
}: Props) {
  const el = useRef<HTMLDivElement>(null);
  const [offer, setOffer] = useState<Offer | null>(null);
  const listId = useId();

  // What is offered for the word the caret is at the end of.
  function suggest() {
    const node = el.current;
    const range = node && caretInside(node);
    if (!node || !range || !range.collapsed || !completions?.length || range.startContainer.nodeType !== Node.TEXT_NODE) return setOffer(null);
    const text = range.startContainer.textContent ?? "";
    const m = WORD_START.exec(text.slice(0, range.startOffset));
    if (!m || WORD_GOES_ON.test(text.slice(range.startOffset))) return setOffer(null);
    const items = completionsFor(m[0], completions);
    if (!items.length) return setOffer(null);
    const r = range.getBoundingClientRect();
    const box = r.height ? r : node.getBoundingClientRect();
    setOffer({ prefix: m[0], items, index: 0, left: box.left, top: box.bottom + 4 });
  }

  // The rest of the chosen word goes in after what was typed, which stays as typed.
  function accept(item: string) {
    const node = el.current;
    if (!node || !offer) return;
    const rest = [...item].slice([...offer.prefix].length).join("");
    setOffer(null);
    node.focus();
    if (!document.execCommand("insertText", false, rest)) {
      const range = caretInside(node);
      if (range) {
        const t = document.createTextNode(rest);
        range.insertNode(t);
        range.setStartAfter(t);
        range.collapse(true);
      }
    }
    onChange(read(node));
  }

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
    <>
      <div
        ref={el}
        className="target-editor"
        role="textbox"
        aria-multiline="true"
        aria-label={label}
        aria-describedby={describedBy}
        aria-autocomplete={completions?.length ? "list" : undefined}
        aria-controls={offer ? listId : undefined}
        aria-activedescendant={offer ? `${listId}-${offer.index}` : undefined}
        contentEditable
        suppressContentEditableWarning
        spellCheck
        lang={lang}
        onInput={() => {
          if (!el.current) return;
          onChange(read(el.current));
          suggest();
        }}
        onBlur={() => setOffer(null)}
        onKeyDown={(e) => {
          if (offer && !e.ctrlKey && !e.metaKey && !e.altKey) {
            if (e.key === "ArrowDown" || e.key === "ArrowUp") {
              e.preventDefault();
              const n = offer.items.length;
              setOffer({ ...offer, index: (offer.index + (e.key === "ArrowDown" ? 1 : n - 1)) % n });
              return;
            }
            if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey)) {
              e.preventDefault();
              accept(offer.items[offer.index] ?? "");
              return;
            }
            if (e.key === "Escape") {
              e.preventDefault();
              e.stopPropagation();
              setOffer(null);
              return;
            }
          }
          if (offer && (e.key === "ArrowLeft" || e.key === "ArrowRight" || e.key === "Home" || e.key === "End")) setOffer(null);
          onKeyDown(e);
        }}
      />
      {offer && (
        <ul id={listId} className="completions" role="listbox" aria-label={tl("Completions")} style={{ left: offer.left, top: offer.top }}>
          {offer.items.map((item, i) => (
            <li
              key={item}
              id={`${listId}-${i}`}
              role="option"
              aria-selected={i === offer.index}
              className={i === offer.index ? "is-selected" : undefined}
              // The editor keeps the caret: the choice is made before it would lose it.
              onMouseDown={(e) => {
                e.preventDefault();
                accept(item);
              }}
            >
              <span className="completion-typed">{[...item].slice(0, [...offer.prefix].length).join("")}</span>
              {[...item].slice([...offer.prefix].length).join("")}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
