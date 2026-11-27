import { useEffect, type ReactNode } from "react";
import { Cloud, HardDrive } from "lucide-react";
import { lang, type LangCode, type Location, type TermStatus } from "../data";
import { useStore } from "../store";
import { tl } from "../i18n";

export function LocationBadge({
  location,
  server,
}: {
  location: Location;
  server?: string;
}) {
  return location === "server" ? (
    <span
      className="badge badge-server"
      title={server ? tl("On server {server}", { server }) : tl("On server")}
    >
      <Cloud size={13} aria-hidden="true" />
      {tl("Server")}
    </span>
  ) : (
    <span className="badge" title={tl("Stored on this computer")}>
      <HardDrive size={13} aria-hidden="true" />
      {tl("Local")}
    </span>
  );
}

const STATUS_LABEL: Record<TermStatus, string> = {
  approved: tl("Approved"),
  proposed: tl("Proposed"),
  forbidden: tl("Forbidden"),
};

export function StatusBadge({ status }: { status: TermStatus }) {
  return (
    <span className={`status status-${status}`}>{STATUS_LABEL[status]}</span>
  );
}

/** Language code chip. The full name is available to assistive technology. */
export function LangChip({ code }: { code: LangCode }) {
  const l = lang(code);
  return (
    <abbr className="lang-chip" title={l.name}>
      {code}
    </abbr>
  );
}

/** Text in a given language, marked up so screen readers and braille switch tables. */
export function InLang({
  code,
  children,
  className,
}: {
  code: LangCode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <span lang={code} dir={lang(code).dir ?? "auto"} className={className}>
      {children}
    </span>
  );
}

/** An entry's terms in every language, each language on a line of its
 * own and every term of it alike: a concept has no source language, and a
 * synonym is as much a term as the first. Forbidden terms stay, struck
 * through. */
export function EntryTerms({
  languages,
  terms,
  big,
}: {
  languages: LangCode[];
  terms: (code: LangCode) => { text: string; status?: string }[];
  big?: boolean;
}) {
  return (
    <span className={`entry-langs${big ? " entry-langs-big" : ""}`}>
      {languages.map((code) => {
        const ts = terms(code);
        return (
          <span key={code} className="entry-lang">
            <LangChip code={code} />
            {ts.length ? (
              <InLang code={code}>
                {ts.map((t, i) => (
                  <span key={i}>
                    {i > 0 && ", "}
                    <span className={`entry-term${t.status === "forbidden" ? " is-forbidden" : ""}`}>{t.text}</span>
                  </span>
                ))}
              </InLang>
            ) : (
              <span className="missing">{tl("missing")}</span>
            )}
          </span>
        );
      })}
    </span>
  );
}

export function PageHeader({
  title,
  children,
  description,
}: {
  title: string;
  children?: ReactNode;
  description?: ReactNode;
}) {
  return (
    <header className="page-header">
      <div>
        <h1 tabIndex={-1} id="page-title">
          {title}
        </h1>
        {description && <p className="page-description">{description}</p>}
      </div>
      {children && <div className="page-actions">{children}</div>}
    </header>
  );
}

export function Toast() {
  const { state, dispatch } = useStore();
  useEffect(() => {
    if (!state.toast) return;
    const t = window.setTimeout(() => dispatch({ type: "clearToast" }), 4000);
    return () => window.clearTimeout(t);
  }, [state.toast, dispatch]);
  return (
    <div className="toast-region" role="status" aria-live="polite">
      {state.toast && (
        <div className="toast" key={state.toast.id}>
          {state.toast.text}
        </div>
      )}
    </div>
  );
}
