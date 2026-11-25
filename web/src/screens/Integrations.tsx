import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Plus } from "lucide-react";
import { useStore } from "../store";
import { useRemote } from "../remote";
import { ApiError, type ServerProvider } from "../server";
import { providerProblem } from "../editor/provider";
import { fmtDate } from "../format";
import { ConfirmButton } from "./TermbaseDialogs";

const text = (e: unknown) => (e instanceof Error ? e.message : String(e));

interface Draft {
  name: string;
  url: string;
  headerName: string;
  headerValue: string;
}

const EMPTY: Draft = { name: "", url: "", headerName: "", headerValue: "" };

type Kind = "translate" | "check";

/** The form for a service: its name, address, and the header that signs requests. */
function ProviderForm({ kind, onSave, taken }: { kind: Kind; onSave: (d: Draft) => Promise<void>; taken: (name: string) => boolean }) {
  const [open, setOpen] = useState(false);
  const [d, setD] = useState<Draft>(EMPTY);
  const [problem, setProblem] = useState("");
  if (!open)
    return (
      <button type="button" className="btn btn-small" onClick={() => setOpen(true)}>
        <Plus size={14} aria-hidden="true" />
        {kind === "check" ? "Add a grammar service" : "Add a service"}
      </button>
    );
  const wrong = providerProblem(d) || (taken(d.name.trim()) ? "A service with this name is there already." : "");
  async function save(e: FormEvent) {
    e.preventDefault();
    if (wrong) return;
    try {
      await onSave({ name: d.name.trim(), url: d.url.trim(), headerName: d.headerName.trim(), headerValue: d.headerValue });
      setD(EMPTY);
      setProblem("");
      setOpen(false);
    } catch (err) {
      setProblem(text(err));
    }
  }
  return (
    <form className="dialog-form provider-form" aria-label={kind === "check" ? "A grammar service" : "A translation service"} onSubmit={save}>
      <div className="field-grid">
        <label className="field">
          <span className="field-label">Name</span>
          <input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
        </label>
        <label className="field">
          <span className="field-label">Address requests are sent to</span>
          <input type="url" placeholder={kind === "check" ? "https://…/v2/check" : "https://"} value={d.url} onChange={(e) => setD({ ...d, url: e.target.value })} />
        </label>
        <label className="field">
          <span className="field-label">
            Header that signs requests <span className="optional">optional</span>
          </span>
          <input placeholder="Authorization" value={d.headerName} onChange={(e) => setD({ ...d, headerName: e.target.value })} />
        </label>
        <label className="field">
          <span className="field-label">
            Its value <span className="optional">optional</span>
          </span>
          <input type="password" autoComplete="off" placeholder="Bearer …" value={d.headerValue} onChange={(e) => setD({ ...d, headerValue: e.target.value })} />
        </label>
      </div>
      {((wrong && d.name && d.url) || problem) && (
        <p className="error" role="alert">
          {problem || wrong}
        </p>
      )}
      <div className="row">
        <button type="submit" className="btn btn-primary btn-small" disabled={!!wrong}>
          Add the service
        </button>
        <button type="button" className="btn btn-quiet btn-small" onClick={() => setOpen(false)}>
          Cancel
        </button>
      </div>
    </form>
  );
}

function ProviderList({ items, onDelete }: { items: { name: string; where: string }[]; onDelete?: (name: string) => void }) {
  if (!items.length) return <p className="muted">No service yet.</p>;
  return (
    <ul className="settings-list">
      {items.map((p) => (
        <li key={p.name} className="row">
          <span>
            <span className="cell-title">{p.name}</span> <span className="muted">{p.where}</span>
          </span>
          {onDelete && <ConfirmButton label="Delete" confirm={`Delete the service ${p.name}?`} action="Delete" onConfirm={() => onDelete(p.name)} />}
        </li>
      ))}
    </ul>
  );
}

const hostOf = (url: string) => {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
};

/**
 * Services at any address that answers the contract: translation services
 * the editor asks for a suggestion (machine translation or a language
 * model), or grammar services that read translations. Those of this
 * browser, and those of the team server.
 */
function Services({ kind }: { kind: Kind }) {
  const { state, dispatch, notify } = useStore();
  const r = useRemote();
  const [onServer, setOnServer] = useState<ServerProvider[] | null>(null);
  const api = r.api;
  const load = useCallback(async () => {
    if (!api) return setOnServer(null);
    try {
      setOnServer(await api.providers());
    } catch {
      setOnServer(null);
    }
  }, [api]);
  useEffect(() => {
    void load();
  }, [load]);
  const admin = !!r.conn?.user.admin;
  const host = r.conn ? new URL(r.conn.url).host : "";
  const check = kind === "check";
  const here = state.providers.filter((p) => (p.kind === "check") === check);
  const id = check ? "s-grammar" : "s-mt";
  return (
    <section className="panel" aria-labelledby={id}>
      <h2 id={id} className="panel-title">
        {check ? "Grammar services" : "Translation services"}
      </h2>
      {check ? (
        <p className="help">
          A grammar checker of your choice, which reads the translation you are working on in a project that names it under Quality checks. Any service works
          that answers a check request at its address as the integration guide describes. In such a project each translation is sent to the service while you
          work on it; in other projects nothing is sent. Spelling stays with the project's dictionary.
        </p>
      ) : (
        <p className="help">
          Machine translation or a language model of your choice, asked for a suggestion in the editor when you ask for one. Any service works that answers at
          an address as the integration guide describes. A segment you ask a suggestion for is sent to the service; nothing is sent on its own.
        </p>
      )}
      {api && <h3 className="field-label">In this browser</h3>}
      <ProviderList
        items={here.map((p) => ({ name: p.name, where: hostOf(p.url) }))}
        onDelete={(name) => {
          dispatch({ type: "deleteProvider", name });
          notify(`Deleted the service ${name}`);
        }}
      />
      <ProviderForm
        kind={kind}
        taken={(name) => state.providers.some((p) => p.name.toLowerCase() === name.toLowerCase())}
        onSave={async (d) => {
          dispatch({ type: "saveProvider", provider: { ...d, kind } });
          notify(`Added the service ${d.name}`);
        }}
      />
      <p className="help">A service kept here is asked by this browser, for projects on this computer, and its key is kept in this browser.</p>
      {api && onServer && (
        <>
          <h3 className="field-label">On {host}</h3>
          <ProviderList
            items={onServer.filter((p) => (p.kind === "check") === check).map((p) => ({ name: p.name, where: p.host }))}
            onDelete={
              admin
                ? (name) =>
                    void api.deleteProvider(name).then(
                      async () => {
                        notify(`Deleted the service ${name} on ${host}`);
                        await load();
                      },
                      (e) => notify(text(e)),
                    )
                : undefined
            }
          />
          {admin ? (
            <ProviderForm
              kind={kind}
              taken={(name) => onServer.some((p) => p.name.toLowerCase() === name.toLowerCase())}
              onSave={async (d) => {
                try {
                  await api.saveProvider(d.name, { kind, url: d.url, headerName: d.headerName, headerValue: d.headerValue });
                } catch (e) {
                  throw e instanceof ApiError ? new Error(e.message) : e;
                }
                notify(`Added the service ${d.name} on ${host}`);
                await load();
              }}
            />
          ) : (
            <p className="help">An administrator adds the services of the server. They are asked by the server, for its projects, and their keys stay there.</p>
          )}
        </>
      )}
    </section>
  );
}

export const TranslationServices = () => <Services kind="translate" />;
export const GrammarServices = () => <Services kind="check" />;

/**
 * Access for programs: a token an assistant or script signs in with on
 * the team server, as the person who made it.
 */
export function ProgramAccess() {
  const { notify } = useStore();
  const r = useRemote();
  const api = r.api;
  const [tokens, setTokens] = useState<{ id: number; name: string; expiresAt: string; current: boolean }[]>([]);
  const [name, setName] = useState("");
  const [days, setDays] = useState(90);
  const [made, setMade] = useState<{ name: string; token: string } | null>(null);
  const [problem, setProblem] = useState("");
  const load = useCallback(async () => {
    if (!api) return;
    try {
      setTokens(await api.ownTokens());
    } catch {
      setTokens([]);
    }
  }, [api]);
  useEffect(() => {
    void load();
  }, [load]);
  if (!api || !r.conn) return null;
  const base = r.conn.url.replace(/\/+$/, "");
  async function create(e: FormEvent) {
    e.preventDefault();
    if (!api || !name.trim()) return;
    try {
      const t = await api.createToken(name.trim(), days);
      setMade({ name: t.name, token: t.token });
      setName("");
      setProblem("");
      await load();
    } catch (err) {
      setProblem(text(err));
    }
  }
  return (
    <section className="panel" aria-labelledby="s-programs">
      <h2 id="s-programs" className="panel-title">
        Access for programs
      </h2>
      <p className="help">
        An AI assistant or a script works on {new URL(base).host} as you, with your rights, when it signs in with a token made here. Assistants that speak the
        Model Context Protocol connect to <code>{base}/mcp</code>.
      </p>
      <ul className="settings-list">
        {tokens.map((t) => (
          <li key={t.id} className="row">
            <span>
              <span className="cell-title">{t.name || "Sign-in"}</span>{" "}
              <span className="muted">
                {t.current ? "this sign-in, " : ""}until {fmtDate(t.expiresAt)}
              </span>
            </span>
            {!t.current && (
              <ConfirmButton
                label="End"
                confirm={`End ${t.name || "this sign-in"}? What signs in with it is refused from then on.`}
                action="End"
                onConfirm={() =>
                  void api.revokeOwnToken(t.id).then(
                    async () => {
                      notify(`Ended ${t.name || "the sign-in"}`);
                      await load();
                    },
                    (e) => notify(text(e)),
                  )
                }
              />
            )}
          </li>
        ))}
      </ul>
      {made && (
        <div className="token-made" role="status">
          <p>
            The token for {made.name}. It is shown this once: copy it now.
          </p>
          <code className="token-value">{made.token}</code>
          <div className="row">
            <button
              type="button"
              className="btn btn-small"
              onClick={() => void navigator.clipboard.writeText(made.token).then(() => notify("Copied the token"), () => notify("The browser did not allow copying; select the token and copy it"))}
            >
              Copy
            </button>
            <button type="button" className="btn btn-quiet btn-small" onClick={() => setMade(null)}>
              Done
            </button>
          </div>
        </div>
      )}
      <form className="role-add" aria-label="A new access token" onSubmit={create}>
        <label className="field">
          <span className="field-label">Program the token is for</span>
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="field">
          <span className="field-label">Lasts</span>
          <select value={days} onChange={(e) => setDays(Number(e.target.value))}>
            <option value={30}>30 days</option>
            <option value={90}>90 days</option>
            <option value={366}>A year</option>
          </select>
        </label>
        <button type="submit" className="btn" disabled={!name.trim()}>
          Make a token
        </button>
        {problem && (
          <p className="error" role="alert">
            {problem}
          </p>
        )}
      </form>
    </section>
  );
}
