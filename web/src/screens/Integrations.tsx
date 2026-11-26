import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { Plus } from "lucide-react";
import { useStore } from "../store";
import { useRemote } from "../remote";
import { ApiError, type ServerProvider } from "../server";
import { localModels, mtProblems, mtServices, providerProblem, type MtInfo, type MtService } from "../editor/provider";
import { fmtDate } from "../format";
import { ConfirmButton } from "./TermbaseDialogs";
import { tl, tlx } from "../i18n";

const text = (e: unknown) => (e instanceof Error ? e.message : String(e));

interface Draft {
  service: MtService;
  name: string;
  url: string;
  headerName: string;
  headerValue: string;
  model: string;
  region: string;
}

const EMPTY: Draft = { service: "own", name: "", url: "", headerName: "", headerValue: "", model: "", region: "" };

type Kind = "translate" | "check";

/** Where the form saves: this browser, or a team server (whose admin may list models with its kept keys). */
interface Target {
  /** Lists the models a key may use; null for a service without models. */
  models: (d: Draft) => Promise<string[] | null>;
  /** Services that can't be asked from here, with why. */
  unavailable?: (i: MtInfo) => string;
}

/**
 * The form for a service. A translation service is one of those built in,
 * set up with the key of one's own account (and a model or a region where
 * it needs one), or a service of one's own at an address; a grammar
 * service is one of one's own.
 */
function ProviderForm({ kind, onSave, taken, target }: { kind: Kind; onSave: (d: Draft) => Promise<void>; taken: (name: string) => boolean; target: Target }) {
  const [open, setOpen] = useState(false);
  const [services, setServices] = useState<MtInfo[]>([]);
  const [d, setD] = useState<Draft>(EMPTY);
  const [problem, setProblem] = useState("");
  const [wrong, setWrong] = useState("");
  const [models, setModels] = useState<string[] | null>(null);
  const [listing, setListing] = useState(false);
  const check = kind === "check";
  useEffect(() => {
    if (!open || check) return;
    let live = true;
    void mtServices().then((list) => {
      if (!live) return;
      setServices(list);
      const first = list.find((i) => !target.unavailable?.(i)) ?? list[0];
      if (first) setD((x) => (x.name ? x : { ...x, service: first.service, name: first.service === "own" ? "" : first.name }));
    });
    return () => {
      live = false;
    };
  }, [open, check, target]);
  const info = services.find((i) => i.service === d.service);
  // What package mt finds wrong, and the name's own problems.
  useEffect(() => {
    if (!open) return;
    let live = true;
    const nameWrong = !d.name.trim() || /[/?#]/.test(d.name) ? tl("Give the service a name, without / ? or #.") : taken(d.name.trim()) ? tl("A service with this name is there already.") : "";
    if (check || d.service === "own") {
      setWrong(nameWrong || providerProblem(d));
      return;
    }
    void mtProblems({ service: d.service, url: d.url, key: d.headerValue, model: d.model, region: d.region }).then((p) => {
      if (live) setWrong(nameWrong || (p[0]?.message ?? ""));
    });
    return () => {
      live = false;
    };
  }, [open, check, d, taken]);
  if (!open)
    return (
      <button type="button" className="btn btn-small" onClick={() => setOpen(true)}>
        <Plus size={14} aria-hidden="true" />
        {check ? tl("Add a grammar service") : tl("Add a service")}
      </button>
    );
  const own = check || d.service === "own";
  const choose = (service: MtService) => {
    const i = services.find((x) => x.service === service);
    const named = services.some((x) => x.name === d.name) || !d.name;
    setD({ ...EMPTY, service, name: named && i && service !== "own" ? i.name : named ? "" : d.name });
    setModels(null);
    setProblem("");
  };
  async function listModels() {
    setListing(true);
    setProblem("");
    try {
      const list = await target.models(d);
      setModels(list ?? []);
      if (list?.length && !d.model) setD((x) => ({ ...x, model: list[0]! }));
    } catch (err) {
      setProblem(text(err));
    } finally {
      setListing(false);
    }
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    if (wrong) return;
    try {
      await onSave({ ...d, name: d.name.trim(), url: d.url.trim(), headerName: d.headerName.trim(), model: d.model.trim(), region: d.region.trim() });
      setD(EMPTY);
      setProblem("");
      setModels(null);
      setOpen(false);
    } catch (err) {
      setProblem(text(err));
    }
  }
  const unavailable = info ? target.unavailable?.(info) : "";
  return (
    <form className="dialog-form provider-form" aria-label={check ? tl("A grammar service") : tl("A translation service")} onSubmit={save}>
      {!check && (
        <label className="field">
          <span className="field-label">{tl("Service")}</span>
          <select value={d.service} onChange={(e) => choose(e.target.value as MtService)}>
            {services.map((i) => (
              <option key={i.service} value={i.service} disabled={!!target.unavailable?.(i)}>
                {i.name}
                {target.unavailable?.(i) ? tl(" (not from this browser)") : ""}
              </option>
            ))}
          </select>
        </label>
      )}
      {info && !check && (
        <p className="help">
          {info.note}{" "}
          {info.signup && (
            <a href={info.signup} target="_blank" rel="noreferrer noopener">
              {tl("Get a key")}
            </a>
          )}
        </p>
      )}
      {unavailable && (
        <p className="error" role="alert">
          {unavailable}
        </p>
      )}
      <div className="field-grid">
        <label className="field">
          <span className="field-label">{tl("Name")}</span>
          <input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
        </label>
        {(own || info?.url) && (
          <label className="field">
            <span className="field-label">
              {own ? tl("Address requests are sent to") : tl("Address of a service that speaks the same API")}
              {!own && <span className="optional">{" "}{tl("optional")}</span>}
            </span>
            <input
              type="url"
              placeholder={check ? "https://…/v2/check" : own ? "https://" : "https://api.openai.com/v1"}
              value={d.url}
              onChange={(e) => setD({ ...d, url: e.target.value })}
            />
          </label>
        )}
        {own ? (
          <>
            <label className="field">
              <span className="field-label">
                {tlx("Header that signs requests {span}", { span: <span className="optional">{tl("optional")}</span> })}</span>
              <input placeholder={tl("Authorization")} value={d.headerName} onChange={(e) => setD({ ...d, headerName: e.target.value })} />
            </label>
            <label className="field">
              <span className="field-label">
                {tlx("Its value {span}", { span: <span className="optional">{tl("optional")}</span> })}</span>
              <input type="password" autoComplete="off" placeholder="Bearer …" value={d.headerValue} onChange={(e) => setD({ ...d, headerValue: e.target.value })} />
            </label>
          </>
        ) : (
          <label className="field">
            <span className="field-label">{tl("Key")}</span>
            <input type="password" autoComplete="off" value={d.headerValue} onChange={(e) => setD({ ...d, headerValue: e.target.value })} />
          </label>
        )}
        {!own && info?.region && (
          <label className="field">
            <span className="field-label">
              {tlx("Region of the key {span}", { span: <span className="optional">{tl("optional for a global key")}</span> })}</span>
            <input placeholder="westeurope" value={d.region} onChange={(e) => setD({ ...d, region: e.target.value })} />
          </label>
        )}
        {!own && info?.model && (
          <div className="field">
            <label className="field-label" htmlFor="provider-model">
              {tl("Model")}
            </label>
            <div className="row row-nowrap">
              <input id="provider-model" className="grow" list="provider-models" value={d.model} onChange={(e) => setD({ ...d, model: e.target.value })} />
              <button type="button" className="btn btn-small" disabled={!d.headerValue || listing} onClick={() => void listModels()}>
                {listing ? tl("Listing…") : tl("List the models")}
              </button>
            </div>
            <datalist id="provider-models">
              {(models ?? []).map((m) => (
                <option key={m} value={m} />
              ))}
            </datalist>
            {models && <span className="help">{models.length ? tl("{length} models the key may use; type to find one.", { length: models.length }) : tl("The key may use no model.")}</span>}
          </div>
        )}
      </div>
      {((wrong && d.name && (own ? d.url : d.headerValue)) || problem) && (
        <p className="error" role="alert">
          {problem || wrong}
        </p>
      )}
      <div className="row">
        <button type="submit" className="btn btn-primary btn-small" disabled={!!wrong || !!unavailable}>
          {tl("Add the service")}
        </button>
        <button type="button" className="btn btn-quiet btn-small" onClick={() => setOpen(false)}>
          {tl("Cancel")}
        </button>
      </div>
    </form>
  );
}

function ProviderList({ items, onDelete }: { items: { name: string; where: string }[]; onDelete?: (name: string) => void }) {
  if (!items.length) return <p className="muted">{tl("No service yet.")}</p>;
  return (
    <ul className="settings-list">
      {items.map((p) => (
        <li key={p.name} className="row">
          <span>
            <span className="cell-title">{p.name}</span> <span className="muted">{p.where}</span>
          </span>
          {onDelete && <ConfirmButton label={tl("Delete")} confirm={tl("Delete the service {name}?", { name: p.name })} action={tl("Delete")} onConfirm={() => onDelete(p.name)} />}
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
  const [labels, setLabels] = useState<Record<string, string>>({});
  useEffect(() => {
    void mtServices().then((list) => setLabels(Object.fromEntries(list.map((i) => [i.service, i.name]))));
  }, []);
  // Where a service is: the address of one's own, or the service built in and its model.
  const where = (service: MtService | undefined, host: string, model?: string) =>
    !service || service === "own" ? host : `${labels[service] ?? service}${model ? `, ${model}` : ""}`;
  const here: Target = useMemo(
    () => ({
      models: (d) => localModels({ service: d.service, url: d.url, key: d.headerValue }),
      unavailable: (i) =>
        i.browser ? "" : tl("{name} does not take requests from a web page. Add it in the desktop app, or on a team server, which asks it for you.", { name: i.name }),
    }),
    [],
  );
  const there: Target = useMemo(
    () => ({
      models: async (d) => (api ? (await api.providerModels({ service: d.service, url: d.url, key: d.headerValue })).models : null),
    }),
    [api],
  );
  const admin = !!r.conn?.user.admin;
  const host = r.conn ? new URL(r.conn.url).host : "";
  const check = kind === "check";
  const local = state.providers.filter((p) => (p.kind === "check") === check);
  const id = check ? "s-grammar" : "s-mt";
  return (
    <section className="panel" aria-labelledby={id}>
      <h2 id={id} className="panel-title">
        {check ? tl("Grammar services") : tl("Translation services")}
      </h2>
      {check ? (
        <p className="help">
          {tl("A grammar checker of your choice, which reads the translation you are working on in a project that names it under Quality checks. Any service works that answers a check request at its address as the integration guide describes. In such a project each translation is sent to the service while you work on it; in other projects nothing is sent. Spelling stays with the project's dictionary.")}
        </p>
      ) : (
        <p className="help">
          {tl("Machine translation or a language model of your choice, asked for a suggestion in the editor when you ask for one: DeepL, Google, Microsoft, OpenAI or Claude with the key of your own account, or a service of your own that answers as the integration guide describes. A segment you ask a suggestion for is sent to the service; nothing is sent on its own.")}
        </p>
      )}
      {api && <h3 className="field-label">{tl("In this browser")}</h3>}
      <ProviderList
        items={local.map((p) => ({ name: p.name, where: where(p.service, hostOf(p.url), p.model) }))}
        onDelete={(name) => {
          dispatch({ type: "deleteProvider", name });
          notify(tl("Deleted the service {name}", { name }));
        }}
      />
      <ProviderForm
        kind={kind}
        taken={(name) => state.providers.some((p) => p.name.toLowerCase() === name.toLowerCase())}
        target={here}
        onSave={async (d) => {
          dispatch({ type: "saveProvider", provider: { ...d, kind } });
          notify(tl("Added the service {name}", { name: d.name }));
        }}
      />
      <p className="help">{tl("A service kept here is asked by this browser, for projects on this computer, and its key is kept in this browser.")}</p>
      {api && onServer && (
        <>
          <h3 className="field-label">{tl("On")}{" "}{host}</h3>
          <ProviderList
            items={onServer.filter((p) => (p.kind === "check") === check).map((p) => ({ name: p.name, where: where(p.service, p.host, p.model) }))}
            onDelete={
              admin
                ? (name) =>
                    void api.deleteProvider(name).then(
                      async () => {
                        notify(tl("Deleted the service {name} on {host}", { name, host }));
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
              target={there}
              onSave={async (d) => {
                try {
                  await api.saveProvider(d.name, {
                    kind,
                    service: kind === "check" ? "own" : d.service,
                    url: d.url,
                    headerName: d.headerName,
                    headerValue: d.headerValue,
                    model: d.model,
                    region: d.region,
                  });
                } catch (e) {
                  throw e instanceof ApiError ? new Error(e.message) : e;
                }
                notify(tl("Added the service {name} on {host}", { name: d.name, host }));
                await load();
              }}
            />
          ) : (
            <p className="help">{tl("An administrator adds the services of the server. They are asked by the server, for its projects, and their keys stay there.")}</p>
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
        {tl("Access for programs")}
      </h2>
      <p className="help">
        {tlx("An AI assistant or a script works on {host} as you, with your rights, when it signs in with a token made here. Assistants that speak the Model Context Protocol connect to {code}.", { host: new URL(base).host, code: <code>{base}/mcp</code> })}
      </p>
      <ul className="settings-list">
        {tokens.map((t) => (
          <li key={t.id} className="row">
            <span>
              <span className="cell-title">{t.name || "Sign-in"}</span>{" "}
              <span className="muted">
                {tlx("{x}until {expiresAt}", { x: t.current ? tl("this sign-in, ") : "", expiresAt: fmtDate(t.expiresAt) })}</span>
            </span>
            {!t.current && (
              <ConfirmButton
                label={tl("End")}
                confirm={(t.name ? tl("End {name}? What signs in with it is refused from then on.", { name: t.name }) : tl("End this sign-in? What signs in with it is refused from then on."))}
                action={tl("End")}
                onConfirm={() =>
                  void api.revokeOwnToken(t.id).then(
                    async () => {
                      notify((t.name ? tl("Ended {name}", { name: t.name }) : tl("Ended the sign-in")));
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
            {tlx("The token for {name}. It is shown this once: copy it now.", { name: made.name })}
          </p>
          <code className="token-value">{made.token}</code>
          <div className="row">
            <button
              type="button"
              className="btn btn-small"
              onClick={() => void navigator.clipboard.writeText(made.token).then(() => notify(tl("Copied the token")), () => notify(tl("The browser did not allow copying; select the token and copy it")))}
            >
              {tl("Copy")}
            </button>
            <button type="button" className="btn btn-quiet btn-small" onClick={() => setMade(null)}>
              {tl("Done")}
            </button>
          </div>
        </div>
      )}
      <form className="role-add" aria-label={tl("A new access token")} onSubmit={create}>
        <label className="field">
          <span className="field-label">{tl("Program the token is for")}</span>
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="field">
          <span className="field-label">{tl("Lasts")}</span>
          <select value={days} onChange={(e) => setDays(Number(e.target.value))}>
            <option value={30}>{tl("30 days")}</option>
            <option value={90}>{tl("90 days")}</option>
            <option value={366}>{tl("A year")}</option>
          </select>
        </label>
        <button type="submit" className="btn" disabled={!name.trim()}>
          {tl("Make a token")}
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
