import { useCallback, useEffect, useState, type FormEvent } from "react";
import { GitPullRequest, RefreshCw } from "lucide-react";
import type { Termbase } from "../data";
import { useStore } from "../store";
import { errorText, useRemote } from "../remote";
import type { MergeRequest } from "../server";
import { fmtDate } from "../format";
import { LocationBadge } from "../components/ui";
import { tl, tlx } from "../i18n";

/** The termbases on the team server the signed-in person may read, in the termbase list. */
export function ServerTermbaseGroup({ current }: { current: string }) {
  const r = useRemote();
  const [list, setList] = useState<{ name: string; kind: string; entries: number }[] | null>(null);
  const load = useCallback(async () => {
    if (!r.api) return;
    try {
      setList(await r.api.termbases());
    } catch {
      setList([]);
    }
  }, [r.api]);
  useEffect(() => {
    void load();
  }, [load]);
  const listen = r.listen;
  useEffect(() => listen((e) => (e.type === "termbase" || e.type === "account") && void load()), [listen, load]);
  if (!r.conn) return null;
  const host = new URL(r.conn.url).host;
  return (
    <div className="tb-group">
      <h2 className="tb-group-title">{tl("On the team server")}</h2>
      {list === null ? (
        <p className="muted tb-group-note">{tl("Loading…")}</p>
      ) : list.length === 0 ? (
        <p className="muted tb-group-note">{tlx("No termbases on {host} for you yet.", { host: host })}</p>
      ) : (
        <ul>
          {list.map((t) => {
            const id = `srv:${t.name}`;
            return (
              <li key={t.name}>
                <a href={`#/terminology/${id}`} aria-current={id === current ? "page" : undefined} className="tb-link">
                  <span className="tb-name">{t.name}</span>
                  <span className="tb-meta">
                    <LocationBadge location="server" server={host} />
                    <span className="tb-count">{t.entries.toLocaleString("en-GB")}</span>
                  </span>
                </a>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

/** Loads a server termbase again, with what others changed. */
export function ReloadButton({ tb }: { tb: Termbase }) {
  const { notify } = useStore();
  const r = useRemote();
  return (
    <button
      type="button"
      className="btn btn-quiet btn-small"
      onClick={() => void r.openTermbase(tb.name).then(() => notify(tl("Loaded {name} again from the server", { name: tb.name })), (e) => notify(errorText(e)))}
    >
      <RefreshCw size={14} aria-hidden="true" />
      {tl("Reload")}
    </button>
  );
}

/**
 * Review on the server: a project termbase's changes go to its masters as
 * merge requests, which the masters' reviewers decide.
 */
export function ServerReviewNote({ tb }: { tb: Termbase }) {
  const { notify } = useStore();
  const r = useRemote();
  const [mrs, setMrs] = useState<MergeRequest[] | null>(null);
  const [master, setMaster] = useState("");
  const [title, setTitle] = useState("");
  const masters = (tb.basedOn ?? []).map((id) => id.replace(/^srv:/, ""));

  const load = useCallback(async () => {
    if (!r.api) return;
    try {
      setMrs(await r.api.mergeRequests(tb.name));
    } catch {
      setMrs([]);
    }
  }, [r.api, tb.name]);
  useEffect(() => {
    void load();
  }, [load]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    try {
      const mr = await r.api!.openMergeRequest(tb.name, master || masters[0]!, title.trim() || tl("{name} into {target}", { name: tb.name, target: master || masters[0]! }));
      notify(tl("Submitted {name} to {target} for review", { name: tb.name, target: mr.target }));
      setTitle("");
      await load();
      window.location.hash = `#/review/${mr.id}`;
    } catch (err) {
      notify(errorText(err));
    }
  }

  return (
    <div className="server-review">
      {mrs && mrs.length > 0 && (
        <ul className="plain-list">
          {mrs.map((m) => (
            <li key={m.id}>
              <a href={`#/review/${m.id}`} className="cell-link">
                <GitPullRequest size={14} aria-hidden="true" /> {m.title}
              </a>{" "}
              <span className="cell-sub">
                {m.state === "open"
                  ? tl("{project} into {target}, open since {createdAt}", { project: m.project, target: m.target, createdAt: fmtDate(m.createdAt) })
                  : tl("{project} into {target}, closed {at}", { project: m.project, target: m.target, at: fmtDate(m.closedAt ?? m.createdAt) })}</span>
            </li>
          ))}
        </ul>
      )}
      {tb.kind === "project" && masters.length > 0 && (
        <form className="role-add" onSubmit={submit} aria-label={tl("Submit for review")}>
          {masters.length > 1 && (
            <label>
              <span className="field-label">{tl("Into")}</span>
              <select value={master || masters[0]} onChange={(e) => setMaster(e.target.value)}>
                {masters.map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </select>
            </label>
          )}
          <label className="field">
            <span className="field-label">{tl("Title of the request")}</span>
            <input value={title} placeholder={tl("{name} into {target}", { name: tb.name, target: master || masters[0]! })} onChange={(e) => setTitle(e.target.value)} />
          </label>
          <button type="submit" className="btn btn-small">
            <GitPullRequest size={14} aria-hidden="true" />{" "}{masters.length > 1 ? tl("Submit to the master for review") : tl("Submit to {name} for review", { name: masters[0] })}
          </button>
        </form>
      )}
    </div>
  );
}
