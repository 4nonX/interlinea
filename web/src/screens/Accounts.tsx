import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { KeyRound, Link2, ShieldCheck, Trash2, UserPlus } from "lucide-react";
import { useStore } from "../store";
import { errorText, useRemote } from "../remote";
import {
  ApiError,
  type CatalogMemory,
  type CatalogTermbase,
  type Grant,
  type Identity,
  PERMISSIONS,
  type Kind,
  type Permission,
  type Role,
  type RoleDef,
  type ServerProjectSummary,
  type ServerUser,
  type SignIn,
} from "../server";
import { fmtDate, plural } from "../format";
import { PageHeader } from "../components/ui";
import { ConfirmButton } from "./TermbaseDialogs";
import { tl, tlx } from "../i18n";

/** What each permission lets someone do, for the role pickers and the roles' table. */
export const ROLE_TEXT: Record<Permission, string> = {
  read: tl("Read"),
  suggest: tl("Suggest changes"),
  edit: tl("Edit"),
  approve: tl("Approve changes"),
  manage: tl("Manage"),
};

/** A role as people read it: the built-in ones in words, the others by their name. */
export function roleText(name: string): string {
  return (ROLE_TEXT as Record<string, string>)[name] ?? name;
}

const KIND_TEXT: Record<Kind, string> = {
  project: tl("Project"),
  memory: tl("Memory"),
  termbase: tl("Termbase"),
  all: tl("Everything"),
};

/** "1 Dec 2026", or "never" for the zero time the server sends for no date. */
function when(t: string | undefined): string {
  return t && !t.startsWith("0001-") ? fmtDate(t) : "never";
}

/**
 * The accounts on the team server and what each may do: administrators make
 * accounts, disable them, set passwords, link provider sign-ins and give
 * roles on projects, memories and termbases.
 */
export function Accounts() {
  const r = useRemote();
  const [users, setUsers] = useState<ServerUser[] | null>(null);
  const [selected, setSelected] = useState<number | null>(null);
  const [problem, setProblem] = useState("");
  const [catalog, setCatalog] = useState<{ memories: CatalogMemory[]; termbases: CatalogTermbase[] }>({ memories: [], termbases: [] });

  const load = useCallback(async () => {
    if (!r.api) return;
    try {
      const [us, cat] = await Promise.all([r.api.users(), r.api.catalog()]);
      setUsers(us);
      setCatalog(cat);
      setProblem("");
    } catch (e) {
      setProblem(errorText(e));
    }
  }, [r.api]);

  useEffect(() => {
    void load();
  }, [load]);
  const listen = r.listen;
  useEffect(() => listen((e) => e.type === "account" && void load()), [listen, load]);

  if (!r.conn)
    return (
      <div className="page page-narrow">
        <PageHeader title={tl("Accounts")} />
        <p>
          {tlx("Accounts are kept on a team server. Sign in to one in {a}.", { a: <a href="#/settings">{tl("Settings")}</a> })}
        </p>
      </div>
    );
  if (!r.conn.user.admin)
    return (
      <div className="page page-narrow">
        <PageHeader title={tl("Accounts")} />
        <p>{tl("Only the server's administrators manage accounts. Managers decide who may use their projects on each project's page.")}</p>
      </div>
    );

  const user = users?.find((u) => u.id === selected) ?? null;
  return (
    <div className="page">
      <PageHeader title={tl("Accounts")} />
      <p className="help">
        {tlx("The people on {host}. A role is a set of permissions: five come with the server, each including the ones before it, and you define others below.", { host: new URL(r.conn.url).host })}
      </p>
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}
      <div className="accounts">
        <section className="panel" aria-labelledby="acc-list">
          <h2 id="acc-list" className="panel-title">
            {users ? plural(users.length, "account", "accounts") : tl("Accounts")}
          </h2>
          {users === null ? (
            <p className="muted" role="status">
              {tl("Loading…")}
            </p>
          ) : (
            <ul className="account-list">
              {users.map((u) => (
                <li key={u.id}>
                  <button
                    type="button"
                    className="account-row"
                    aria-current={u.id === selected ? "true" : undefined}
                    onClick={() => setSelected(u.id)}
                  >
                    <span className="cell-title">{u.name || u.username}</span>
                    <span className="cell-sub">
                      {[u.username, u.admin ? tl("administrator") : "", u.disabled ? tl("disabled") : "", !u.hasPassword ? tl("provider sign-in only") : ""].filter(Boolean).join(", ")}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          <NewAccount
            onMade={async (u) => {
              await load();
              setSelected(u.id);
            }}
          />
        </section>
        {user ? (
          <AccountDetail key={user.id} user={user} catalog={catalog} projects={r.projects ?? []} onChanged={load} onDeleted={async () => {
            setSelected(null);
            await load();
          }} />
        ) : (
          <section className="panel account-empty">
            <p className="muted">{tl("Choose an account to see and change it.")}</p>
          </section>
        )}
      </div>
      <RoleDefinitions />
    </div>
  );
}

function NewAccount({ onMade }: { onMade: (u: ServerUser) => Promise<void> }) {
  const { notify } = useStore();
  const r = useRemote();
  const blank = { username: "", name: "", email: "", password: "", admin: false };
  const [u, setU] = useState(blank);
  const [problems, setProblems] = useState<Record<string, string>>({});

  async function make(e: FormEvent) {
    e.preventDefault();
    setProblems({});
    try {
      const made = await r.api!.createUser(u);
      notify(tl("Made the account {username}", { username: made.username }));
      setU(blank);
      await r.refresh();
      await onMade(made);
    } catch (err) {
      if (err instanceof ApiError && err.problems.length)
        setProblems(Object.fromEntries(err.problems.map((p) => [p.field, p.message])));
      else if (err instanceof ApiError && err.code === "exists") setProblems({ username: err.message });
      else setProblems({ form: errorText(err) });
    }
  }

  return (
    <form className="server-form account-new" onSubmit={make} aria-label={tl("New account")}>
      <h3 className="field-label">{tl("New account")}</h3>
      <label className="field">
        <span className="field-label">{tl("Username")}</span>
        <input value={u.username} autoComplete="off" onChange={(e) => setU({ ...u, username: e.target.value })} aria-invalid={!!problems.username} />
        {problems.username && <span className="error">{problems.username}</span>}
      </label>
      <label className="field">
        <span className="field-label">{tl("Name")}</span>
        <input value={u.name} onChange={(e) => setU({ ...u, name: e.target.value })} />
      </label>
      <label className="field">
        <span className="field-label">{tl("Email")}</span>
        <input type="email" value={u.email} onChange={(e) => setU({ ...u, email: e.target.value })} aria-invalid={!!problems.email} />
        {problems.email && <span className="error">{problems.email}</span>}
      </label>
      <label className="field">
        <span className="field-label">{tl("Password")}</span>
        <input type="password" value={u.password} autoComplete="new-password" onChange={(e) => setU({ ...u, password: e.target.value })} aria-invalid={!!problems.password} />
        <span className="cell-sub">{tl("12 characters or more. Leave it empty for someone who signs in only through the identity provider.")}</span>
        {problems.password && <span className="error">{problems.password}</span>}
      </label>
      <label className="check">
        <input type="checkbox" checked={u.admin} onChange={(e) => setU({ ...u, admin: e.target.checked })} />{" "}{tl("Administrator")}
      </label>
      {problems.form && (
        <p className="error" role="alert">
          {problems.form}
        </p>
      )}
      <div className="server-actions">
        <button type="submit" className="btn btn-primary" disabled={!u.username.trim()}>
          <UserPlus size={15} aria-hidden="true" />{" "}{tl("Make the account")}
        </button>
      </div>
    </form>
  );
}

function AccountDetail({
  user,
  catalog,
  projects,
  onChanged,
  onDeleted,
}: {
  user: ServerUser;
  catalog: { memories: CatalogMemory[]; termbases: CatalogTermbase[] };
  projects: ServerProjectSummary[];
  onChanged: () => Promise<void>;
  onDeleted: () => Promise<void>;
}) {
  const { notify } = useStore();
  const r = useRemote();
  const api = r.api!;
  const me = r.conn!.user.id === user.id;
  const [d, setD] = useState({ name: user.name, email: user.email ?? "" });
  const [password, setPassword] = useState("");
  const [signIns, setSignIns] = useState<SignIn[]>([]);
  const [identities, setIdentities] = useState<Identity[]>([]);
  const [grants, setGrants] = useState<Grant[]>([]);
  const [problem, setProblem] = useState("");

  const load = useCallback(async () => {
    try {
      const [ts, ids, gs] = await Promise.all([api.userTokens(user.id), api.userIdentities(user.id), api.userGrants(user.id)]);
      setSignIns(ts);
      setIdentities(ids);
      setGrants(gs);
    } catch (e) {
      setProblem(errorText(e));
    }
  }, [api, user.id]);
  useEffect(() => {
    void load();
  }, [load]);

  const attempt = async (what: () => Promise<unknown>, done: string) => {
    setProblem("");
    try {
      await what();
      notify(done);
      await onChanged();
      await load();
    } catch (e) {
      setProblem(
        e instanceof ApiError && e.code === "lastAdmin"
          ? tl("The server needs at least one active administrator: make someone else one first.")
          : e instanceof ApiError && e.problems.length
            ? e.problems.map((p) => p.message).join(" ")
            : errorText(e),
      );
    }
  };

  const projectName = (id: string) => projects.find((p) => String(p.id) === id)?.settings.name ?? tl("Project {id}", { id });
  const resourceName = (g: Grant) =>
    g.kind === "all" ? tl("Everything") : g.resource === "*" ? (g.kind === "memory" ? tl("All memories") : g.kind === "project" ? tl("All projects") : tl("All termbases")) : g.kind === "project" ? projectName(g.resource) : g.resource;

  return (
    <section className="panel account-detail" aria-labelledby="acc-detail">
      <h2 id="acc-detail" className="panel-title">
        {user.name || user.username}
      </h2>
      <p className="cell-sub">
        {(me ? tl("{username}, made on {createdAt}. This is you.", { username: user.username, createdAt: fmtDate(user.createdAt) }) : tl("{username}, made on {createdAt}.", { username: user.username, createdAt: fmtDate(user.createdAt) }))}</p>
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}

      <form
        className="server-form"
        aria-label={tl("Details")}
        onSubmit={(e) => {
          e.preventDefault();
          void attempt(() => api.updateUser(user, { name: d.name.trim(), email: d.email.trim() }), tl("Saved the details"));
        }}
      >
        <label className="field">
          <span className="field-label">{tl("Name")}</span>
          <input value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
        </label>
        <label className="field">
          <span className="field-label">{tl("Email")}</span>
          <input type="email" value={d.email} onChange={(e) => setD({ ...d, email: e.target.value })} />
        </label>
        <div className="server-actions">
          <button type="submit" className="btn" disabled={d.name === user.name && d.email === (user.email ?? "")}>
            {tl("Save details")}
          </button>
        </div>
      </form>

      <div className="account-switches">
        <label className="check">
          <input
            type="checkbox"
            checked={user.admin}
            onChange={(e) =>
              void attempt(() => api.updateUser(user, { admin: e.target.checked }), e.target.checked ? tl("{username} is an administrator", { username: user.username }) : tl("{username} is no longer an administrator", { username: user.username }))
            }
          />{" "}
          <ShieldCheck size={15} aria-hidden="true" />{" "}{tl("Administrator: may do everything on the server")}
        </label>
        <label className="check">
          <input
            type="checkbox"
            checked={user.disabled}
            onChange={(e) =>
              void attempt(() => api.updateUser(user, { disabled: e.target.checked }), e.target.checked ? tl("Disabled {username} and signed them out everywhere", { username: user.username }) : tl("Enabled {username}", { username: user.username }))
            }
          />{" "}
          {tl("Disabled: can't sign in; signed out everywhere")}
        </label>
      </div>

      <h3 className="field-label">{tl("Password")}</h3>
      <form
        className="server-connect"
        onSubmit={(e) => {
          e.preventDefault();
          void attempt(async () => {
            await api.setUserPassword(user.id, password);
            setPassword("");
          }, tl("Set a new password for {username}; their sign-ins ended", { username: user.username }));
        }}
      >
        <label className="field">
          <span className="field-label">{user.hasPassword ? tl("New password") : tl("Password (the account has none)")}</span>
          <input type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        <button type="submit" className="btn" disabled={password.length === 0}>
          <KeyRound size={15} aria-hidden="true" />{" "}{tl("Set password")}
        </button>
      </form>
      {user.hasPassword && identities.length > 0 && (
        <ConfirmButton
          label={tl("Remove the password")}
          confirm={tl("{username} then signs in only through the identity provider.", { username: user.username })}
          action={tl("Remove")}
          onConfirm={() => void attempt(() => api.setUserPassword(user.id, ""), tl("{username} signs in through the provider only", { username: user.username }))}
        />
      )}

      <h3 className="field-label">{tl("Sign-ins")}</h3>
      {signIns.length === 0 ? (
        <p className="muted">{tl("Not signed in anywhere.")}</p>
      ) : (
        <ul className="plain-list">
          {signIns.map((t) => (
            <li key={t.id} className="cell-sub">
              {tlx("{name}: since {createdAt}, last used {lastUsed}, ends {expiresAt}", { name: t.name, createdAt: fmtDate(t.createdAt), lastUsed: when(t.lastUsed), expiresAt: fmtDate(t.expiresAt) })}</li>
          ))}
        </ul>
      )}
      {signIns.length > 0 && (
        <button type="button" className="btn btn-small" onClick={() => void attempt(() => api.revokeUserTokens(user.id), tl("Signed {username} out everywhere", { username: user.username }))}>
          {tl("Sign out everywhere")}
        </button>
      )}

      <Identities user={user} identities={identities} attempt={attempt} />

      <Roles user={user} grants={grants} resourceName={resourceName} catalog={catalog} projects={projects} attempt={attempt} />

      {!me && (
        <div className="account-delete">
          <ConfirmButton
            small={false}
            label={
              <>
                <Trash2 size={15} aria-hidden="true" />{" "}{tl("Delete the account")}
              </>
            }
            confirm={tl("Delete {username}? Their work stays signed with their name; their roles and sign-ins go.", { username: user.username })}
            action={tl("Delete")}
            onConfirm={() =>
              void api.deleteUser(user.id).then(
                async () => {
                  notify(tl("Deleted the account {username}", { username: user.username }));
                  await onDeleted();
                },
                (e) => setProblem(errorText(e)),
              )
            }
          />
        </div>
      )}
    </section>
  );
}

type Attempt = (what: () => Promise<unknown>, done: string) => Promise<void>;

function Identities({ user, identities, attempt }: { user: ServerUser; identities: Identity[]; attempt: Attempt }) {
  const r = useRemote();
  const [subject, setSubject] = useState("");
  if (!r.info?.provider && identities.length === 0) return null;
  return (
    <>
      <h3 className="field-label">{r.info?.provider ? tl("Sign-in through {provider}", { provider: r.info.provider }) : tl("Sign-in through the identity provider")}</h3>
      {identities.length === 0 ? (
        <p className="muted">{tlx("Not linked. {username} signs in with a password, or links it themselves after signing in.", { username: user.username })}</p>
      ) : (
        <ul className="plain-list">
          {identities.map((i) => (
            <li key={i.issuer + i.subject} className="identity">
              <span className="cell-sub">
                {tlx("{host}, subject {subject}", { host: new URL(i.issuer).host, subject: i.subject })}</span>
              <button
                type="button"
                className="btn btn-quiet btn-small"
                onClick={() => void attempt(() => r.api!.unlinkIdentity(user.id, i.issuer, i.subject), tl("Unlinked the sign-in"))}
              >
                {tl("Unlink")}
              </button>
            </li>
          ))}
        </ul>
      )}
      {r.info?.provider && (
        <form
          className="server-connect"
          onSubmit={(e) => {
            e.preventDefault();
            void attempt(async () => {
              await r.api!.linkIdentity(user.id, "", subject.trim());
              setSubject("");
            }, tl("Linked {username} to the sign-in", { username: user.username }));
          }}
        >
          <label className="field">
            <span className="field-label">{tlx("Subject at {provider} (its ID for the person)", { provider: r.info.provider })}</span>
            <input value={subject} onChange={(e) => setSubject(e.target.value)} />
          </label>
          <button type="submit" className="btn" disabled={!subject.trim()}>
            <Link2 size={15} aria-hidden="true" />{" "}{tl("Link")}
          </button>
        </form>
      )}
    </>
  );
}

function Roles({
  user,
  grants,
  resourceName,
  catalog,
  projects,
  attempt,
}: {
  user: ServerUser;
  grants: Grant[];
  resourceName: (g: Grant) => string;
  catalog: { memories: CatalogMemory[]; termbases: CatalogTermbase[] };
  projects: ServerProjectSummary[];
  attempt: Attempt;
}) {
  const r = useRemote();
  const [kind, setKind] = useState<Kind>("project");
  const [resource, setResource] = useState("*");
  const [role, setRole] = useState<Role>("edit");
  const choices = useMemo(() => {
    switch (kind) {
      case "project":
        return projects.map((p) => ({ value: String(p.id), label: p.settings.name }));
      case "memory":
        return catalog.memories.map((m) => ({ value: m.name, label: m.name }));
      case "termbase":
        return catalog.termbases.map((t) => ({ value: t.name, label: t.name }));
      case "all":
        return [];
    }
  }, [kind, projects, catalog]);

  return (
    <>
      <h3 className="field-label">{tl("Roles")}</h3>
      {user.admin && <p className="cell-sub">{tlx("As an administrator, {username} may do everything; roles add nothing.", { username: user.username })}</p>}
      {grants.length === 0 ? (
        <p className="muted">{tlx("No roles. {username} sees only the projects with work given to them.", { username: user.username })}</p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th scope="col">{tl("On")}</th>
              <th scope="col">{tl("Role")}</th>
              <th scope="col">
                <span className="sr-only">{tl("Remove")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {grants.map((g) => (
              <tr key={g.kind + g.resource}>
                <th scope="row">
                  <span className="cell-title">{resourceName(g)}</span>
                  <div className="cell-sub">{KIND_TEXT[g.kind]}</div>
                </th>
                <td>
                  <select
                    className="select-small"
                    value={g.role}
                    aria-label={tl("Role on {g}", { g: resourceName(g) })}
                    onChange={(e) => void attempt(() => r.api!.setGrant({ ...g, role: e.target.value as Role }), tl("Changed the role"))}
                  >
                    {r.roles.map((x) => (
                      <option key={x.name} value={x.name}>
                        {roleText(x.name)}
                      </option>
                    ))}
                  </select>
                </td>
                <td className="cell-action">
                  <button type="button" className="btn btn-quiet btn-small" onClick={() => void attempt(() => r.api!.revokeGrant(g), tl("Took the role away"))}>
                    {tl("Remove")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <form
        className="role-add"
        aria-label={tl("Give a role")}
        onSubmit={(e) => {
          e.preventDefault();
          void attempt(() => r.api!.setGrant({ userId: user.id, kind, resource: kind === "all" ? "*" : resource, role }), tl("Gave the role"));
        }}
      >
        <label>
          <span className="field-label">{tl("On")}</span>
          <select
            value={kind}
            onChange={(e) => {
              setKind(e.target.value as Kind);
              setResource("*");
            }}
          >
            <option value="project">{tl("Projects")}</option>
            <option value="memory">{tl("Memories")}</option>
            <option value="termbase">{tl("Termbases")}</option>
            <option value="all">{tl("Everything")}</option>
          </select>
        </label>
        {kind !== "all" && (
          <label>
            <span className="field-label">{tl("Which")}</span>
            <select value={resource} onChange={(e) => setResource(e.target.value)}>
              <option value="*">{tl("All of them, also new ones")}</option>
              {choices.map((c) => (
                <option key={c.value} value={c.value}>
                  {c.label}
                </option>
              ))}
            </select>
          </label>
        )}
        <label>
          <span className="field-label">{tl("Role")}</span>
          <select value={role} onChange={(e) => setRole(e.target.value as Role)}>
            {r.roles.map((x) => (
              <option key={x.name} value={x.name}>
                {roleText(x.name)}
              </option>
            ))}
          </select>
        </label>
        <button type="submit" className="btn">
          {tl("Give the role")}
        </button>
      </form>
      <p className="cell-sub">
        {tl("Manage on all projects makes someone a project manager: they make projects and give out work. Manage on all memories or termbases lets them make those.")}
      </p>
    </>
  );
}

/**
 * The roles themselves: what each allows. The five that come with the
 * server are fixed; administrators define others as sets of permissions,
 * change them, rename them and delete those nobody has.
 */
function RoleDefinitions() {
  const { notify } = useStore();
  const r = useRemote();
  const api = r.api!;
  const [name, setName] = useState("");
  const [perms, setPerms] = useState<Permission[]>(["read"]);
  const [problem, setProblem] = useState("");

  const attempt = async (what: () => Promise<unknown>, done: string) => {
    setProblem("");
    try {
      await what();
      notify(done);
      await r.refresh();
    } catch (e) {
      setProblem(e instanceof ApiError && e.problems.length ? e.problems.map((p) => p.message).join(" ") : errorText(e));
    }
  };
  const toggled = (have: Permission[], p: Permission, on: boolean) => PERMISSIONS.filter((x) => (x === p ? on : have.includes(x)));

  return (
    <section className="panel" aria-labelledby="role-defs">
      <h2 id="role-defs" className="panel-title">
        {tl("Roles")}
      </h2>
      <p className="help">
        {tl("What each role allows. The first five come with the server. Define your own for other combinations, such as a reviewer who suggests and approves but does not edit. A change applies at once to everyone who has the role.")}
      </p>
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}
      <div className="table-wrap">
        <table className="table role-table">
          <thead>
            <tr>
              <th scope="col">{tl("Role")}</th>
              {PERMISSIONS.map((p) => (
                <th key={p} scope="col">
                  {ROLE_TEXT[p]}
                </th>
              ))}
              <th scope="col" className="num">
                {tl("Given")}
              </th>
              <th scope="col">
                <span className="sr-only">{tl("Delete")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {r.roles.map((def) => (
              <RoleRow key={def.name} def={def} attempt={attempt} toggled={toggled} />
            ))}
          </tbody>
        </table>
      </div>
      <form
        className="role-add"
        aria-label={tl("New role")}
        onSubmit={(e) => {
          e.preventDefault();
          void attempt(async () => {
            await api.setRole(name.trim(), perms);
            setName("");
            setPerms(["read"]);
          }, tl("Made the role {name}", { name: name.trim() }));
        }}
      >
        <label>
          <span className="field-label">{tl("New role")}</span>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder={tl("For example Reviewer")} />
        </label>
        <fieldset className="role-perms">
          <legend className="field-label">{tl("It allows")}</legend>
          {PERMISSIONS.map((p) => (
            <label key={p} className="check">
              <input type="checkbox" checked={perms.includes(p)} disabled={p === "read"} onChange={(e) => setPerms(toggled(perms, p, e.target.checked))} /> {ROLE_TEXT[p]}
            </label>
          ))}
        </fieldset>
        <button type="submit" className="btn" disabled={!name.trim()}>
          {tl("Make the role")}
        </button>
      </form>
    </section>
  );
}

function RoleRow({
  def,
  attempt,
  toggled,
}: {
  def: RoleDef;
  attempt: (what: () => Promise<unknown>, done: string) => Promise<void>;
  toggled: (have: Permission[], p: Permission, on: boolean) => Permission[];
}) {
  const r = useRemote();
  const api = r.api!;
  const [name, setName] = useState(def.name);
  useEffect(() => setName(def.name), [def.name]);
  const rename = () => {
    const v = name.trim();
    if (!v || v === def.name) {
      setName(def.name);
      return;
    }
    void attempt(() => api.setRole(def.name, def.permissions, v), tl("Renamed the role to {v}", { v })).then(() => setName((n) => (n === v ? n : def.name)));
  };
  return (
    <tr>
      <th scope="row">
        {def.builtIn ? (
          <>
            <span className="cell-title">{roleText(def.name)}</span>
            <div className="cell-sub">{tl("Comes with the server")}</div>
          </>
        ) : (
          <input
            value={name}
            aria-label={tl("Name of the role {name}", { name: def.name })}
            onChange={(e) => setName(e.target.value)}
            onBlur={rename}
            onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), rename())}
          />
        )}
      </th>
      {PERMISSIONS.map((p) => (
        <td key={p}>
          <input
            type="checkbox"
            aria-label={`${roleText(def.name)}: ${ROLE_TEXT[p]}`}
            checked={def.permissions.includes(p)}
            disabled={def.builtIn || p === "read"}
            onChange={(e) =>
              void attempt(
                () => api.setRole(def.name, toggled(def.permissions, p, e.target.checked)),
                `${def.name} ${e.target.checked ? tl("now allows") : tl("no longer allows")}: ${ROLE_TEXT[p].toLowerCase()}`,
              )
            }
          />
        </td>
      ))}
      <td className="num">{def.grants}</td>
      <td className="cell-action">
        {!def.builtIn && (
          <ConfirmButton
            label={tl("Delete")}
            disabled={def.grants > 0}
            confirm={tl("Delete the role {name}?", { name: def.name })}
            action={tl("Delete")}
            onConfirm={() => void attempt(() => api.deleteRole(def.name), tl("Deleted the role {name}", { name: def.name }))}
          />
        )}
      </td>
    </tr>
  );
}
