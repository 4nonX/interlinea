import { useCallback, useEffect, useState, type FormEvent } from "react";
import { GitPullRequest, RefreshCw } from "lucide-react";
import type { Termbase } from "../data";
import { useStore } from "../store";
import { errorText, useRemote } from "../remote";
import type { MergeRequest } from "../server";
import { fmtDate } from "../format";
import { LocationBadge } from "../components/ui";

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
      <h2 className="tb-group-title">On the team server</h2>
      {list === null ? (
        <p className="muted tb-group-note">Loading…</p>
      ) : list.length === 0 ? (
        <p className="muted tb-group-note">No termbases on {host} for you yet.</p>
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
      onClick={() => void r.openTermbase(tb.name).then(() => notify(`Loaded ${tb.name} again from the server`), (e) => notify(errorText(e)))}
    >
      <RefreshCw size={14} aria-hidden="true" />
      Reload
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
      const mr = await r.api!.openMergeRequest(tb.name, master || masters[0]!, title.trim() || `${tb.name} into ${master || masters[0]}`);
      notify(`Submitted ${tb.name} to ${mr.target} for review`);
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
                {m.project} into {m.target}, {m.state === "open" ? `open since ${fmtDate(m.createdAt)}` : `closed ${fmtDate(m.closedAt ?? m.createdAt)}`}
              </span>
            </li>
          ))}
        </ul>
      )}
      {tb.kind === "project" && masters.length > 0 && (
        <form className="role-add" onSubmit={submit} aria-label="Submit for review">
          {masters.length > 1 && (
            <label>
              <span className="field-label">Into</span>
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
            <span className="field-label">Title of the request</span>
            <input value={title} placeholder={`${tb.name} into ${master || masters[0]}`} onChange={(e) => setTitle(e.target.value)} />
          </label>
          <button type="submit" className="btn btn-small">
            <GitPullRequest size={14} aria-hidden="true" /> Submit to {masters.length > 1 ? "the master" : masters[0]} for review
          </button>
        </form>
      )}
    </div>
  );
}
