import { useEffect, useState, type FormEvent } from "react";
import { Cloud, LogIn, LogOut } from "lucide-react";
import { useStore } from "../store";
import { errorText, useRemote } from "../remote";
import { ApiError, serverURL, type ServerInfo } from "../server";
import { fmtDate } from "../format";
import { tl, tlx } from "../i18n";

/** Settings: the team server the projects are shared on, and signing in to it. */
export function TeamServer() {
  const { notify } = useStore();
  const r = useRemote();
  const [url, setUrl] = useState("");
  const [info, setInfo] = useState<ServerInfo | null>(null);
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [setup, setSetup] = useState({ setupToken: "", username: "", name: "", email: "", password: "" });

  // Served by a team server, the web app offers that server.
  useEffect(() => {
    if (r.conn || url) return;
    let live = true;
    void r.check(window.location.origin).then(
      (i) => {
        if (!live) return;
        setUrl(window.location.origin);
        setInfo(i);
      },
      () => undefined,
    );
    return () => {
      live = false;
    };
    // Once, when the page opens signed out.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [r.conn]);

  const target = serverURL(url);

  async function check(e: FormEvent) {
    e.preventDefault();
    setProblem("");
    setInfo(null);
    if (!target) {
      setProblem(tl("Type the server's address."));
      return;
    }
    setBusy(true);
    try {
      setInfo(await r.check(target));
    } catch (err) {
      setProblem(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  async function signIn(e: FormEvent) {
    e.preventDefault();
    setProblem("");
    setBusy(true);
    try {
      await r.signIn(target, username, password);
      setPassword("");
      notify(tl("Signed in to {host}", { host: new URL(target).host }));
    } catch (err) {
      setProblem(
        err instanceof ApiError && err.code === "tooMany"
          ? err.message
          : errorText(err),
      );
    } finally {
      setBusy(false);
    }
  }

  async function makeAdmin(e: FormEvent) {
    e.preventDefault();
    setProblem("");
    setBusy(true);
    try {
      await r.setup(target, setup);
      notify(tl("The server is set up. Signed in as {username}", { username: setup.username }));
    } catch (err) {
      setProblem(
        err instanceof ApiError && err.problems.length
          ? err.problems.map((p) => p.message).join(" ")
          : errorText(err),
      );
    } finally {
      setBusy(false);
    }
  }

  async function provider() {
    setProblem("");
    try {
      await r.signInWithProvider(target);
    } catch (err) {
      setProblem(errorText(err));
    }
  }

  return (
    <section className="panel" aria-labelledby="s-servers">
      <h2 id="s-servers" className="panel-title">
        {tl("Team server")}
      </h2>
      {r.conn ? (
        <ul className="server-list">
          <li>
            <Cloud size={18} aria-hidden="true" />
            <div>
              <div className="cell-title">{new URL(r.conn.url).host}</div>
              <div className="cell-sub">
                {(r.conn.user.admin ? tl("Signed in as {name} ({username}), administrator. The sign-in lasts until {expiresAt}.", { name: r.conn.user.name, username: r.conn.user.username, expiresAt: fmtDate(r.conn.expiresAt) }) : tl("Signed in as {name} ({username}). The sign-in lasts until {expiresAt}.", { name: r.conn.user.name, username: r.conn.user.username, expiresAt: fmtDate(r.conn.expiresAt) })) + (r.info ? tl(" Server version {version}.", { version: r.info.version }) : "")}</div>
              {r.unsaved > 0 && (
                <div className="cell-sub cell-warn" role="status">
                  {r.unsaved === 1
                    ? tl("1 translation is not on the server yet; it is sent again when the server answers.")
                    : tl("{unsaved} translations are not on the server yet; they are sent again when the server answers.", { unsaved: r.unsaved })}
                </div>
              )}
              {r.error && (
                <div className="cell-sub cell-warn" role="status">
                  {r.error}
                </div>
              )}
            </div>
            <span className={`status ${r.live ? "status-approved" : "status-proposed"}`}>
              {r.live ? tl("Connected") : tl("Reconnecting")}
            </span>
            <button type="button" className="btn btn-small" onClick={() => void r.signOut()}>
              <LogOut size={15} aria-hidden="true" />{" "}{tl("Sign out")}
            </button>
          </li>
        </ul>
      ) : (
        <>
          <p className="help">
            {tl("A team server keeps a team's termbases in one place: everyone translates with the same terms, and new and changed entries reach the masters through review. Projects and memories can be shared there as well, with work given to translators and reviewers.")}
          </p>
          <form className="server-connect" onSubmit={check}>
            <label className="field">
              <span className="field-label">{tl("Server address")}</span>
              <input
                type="url"
                inputMode="url"
                value={url}
                placeholder="https://interlinea.example.com"
                onChange={(e) => {
                  setUrl(e.target.value);
                  setInfo(null);
                }}
                aria-describedby={problem ? "server-problem" : undefined}
              />
            </label>
            <button type="submit" className="btn" disabled={busy}>
              {tl("Connect")}
            </button>
          </form>
          {info && info.setup && (
            <form className="server-form" onSubmit={makeAdmin} aria-label={tl("Set up the server")}>
              <p className="help">
                {tlx("{host} has no accounts yet. Make the first administrator with the setup token from the server's log.", { host: new URL(target).host })}
              </p>
              <label className="field">
                <span className="field-label">{tl("Setup token")}</span>
                <input
                  value={setup.setupToken}
                  autoComplete="off"
                  onChange={(e) => setSetup({ ...setup, setupToken: e.target.value })}
                />
              </label>
              <label className="field">
                <span className="field-label">{tl("Username")}</span>
                <input
                  value={setup.username}
                  autoComplete="username"
                  onChange={(e) => setSetup({ ...setup, username: e.target.value })}
                />
              </label>
              <label className="field">
                <span className="field-label">{tl("Name")}</span>
                <input value={setup.name} onChange={(e) => setSetup({ ...setup, name: e.target.value })} />
              </label>
              <label className="field">
                <span className="field-label">{tl("Email")}</span>
                <input type="email" value={setup.email} onChange={(e) => setSetup({ ...setup, email: e.target.value })} />
              </label>
              <label className="field">
                <span className="field-label">{tl("Password (12 characters or more)")}</span>
                <input
                  type="password"
                  value={setup.password}
                  autoComplete="new-password"
                  onChange={(e) => setSetup({ ...setup, password: e.target.value })}
                />
              </label>
              <button type="submit" className="btn btn-primary" disabled={busy}>
                {tl("Make the administrator")}
              </button>
            </form>
          )}
          {info && !info.setup && (
            <form className="server-form" onSubmit={signIn} aria-label={tl("Sign in to {host}", { host: new URL(target).host })}>
              <label className="field">
                <span className="field-label">{tl("Username")}</span>
                <input value={username} autoComplete="username" onChange={(e) => setUsername(e.target.value)} />
              </label>
              <label className="field">
                <span className="field-label">{tl("Password")}</span>
                <input
                  type="password"
                  value={password}
                  autoComplete="current-password"
                  onChange={(e) => setPassword(e.target.value)}
                />
              </label>
              <div className="server-actions">
                <button type="submit" className="btn btn-primary" disabled={busy || !username || !password}>
                  <LogIn size={15} aria-hidden="true" />{" "}{tl("Sign in")}
                </button>
                {info.provider && (
                  <button type="button" className="btn" onClick={() => void provider()}>
                    {tlx("Sign in with {provider}", { provider: info.provider })}</button>
                )}
              </div>
            </form>
          )}
          {problem && (
            <p id="server-problem" className="error" role="alert">
              {problem}
            </p>
          )}
        </>
      )}
    </section>
  );
}
