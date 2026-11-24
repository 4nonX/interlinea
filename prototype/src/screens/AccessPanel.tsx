import { useCallback, useEffect, useState, type FormEvent } from "react";
import { errorText, useRemote } from "../remote";
import type { GrantView, Kind, Role } from "../server";
import { roleText } from "./Accounts";

/**
 * Who may use a project, memory or termbase on the team server, for its
 * managers and the administrators: roles given, changed and taken away.
 */
export function AccessPanel({
  kind,
  resource,
  title,
  embedded = false,
}: {
  kind: Exclude<Kind, "all">;
  resource: string;
  title: string;
  /** A section of a dialog: no card around it, and its title as the dialog's other sections have theirs. */
  embedded?: boolean;
}) {
  const r = useRemote();
  const [grants, setGrants] = useState<GrantView[] | null>(null);
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState<Role>(kind === "project" ? "edit" : "read");
  const [problem, setProblem] = useState("");

  const load = useCallback(async () => {
    if (!r.api) return;
    try {
      setGrants(await r.api.grantsOn(kind, resource));
    } catch (e) {
      setProblem(errorText(e));
    }
  }, [r.api, kind, resource]);
  useEffect(() => {
    void load();
  }, [load]);

  const attempt = async (what: () => Promise<unknown>) => {
    setProblem("");
    try {
      await what();
      await load();
    } catch (e) {
      setProblem(errorText(e));
    }
  };

  function give(e: FormEvent) {
    e.preventDefault();
    void attempt(async () => {
      await r.api!.setGrant({ userId: Number(userId), kind, resource, role });
      setUserId("");
    });
  }

  const id = `access-${kind}`;
  return (
    <section className={embedded ? "panel panel-embedded" : "panel"} aria-labelledby={id}>
      {embedded ? (
        <h3 id={id} className="settings-title">
          {title}
        </h3>
      ) : (
        <h2 id={id} className="panel-title">
          {title}
        </h2>
      )}
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}
      {grants === null ? (
        <p className="muted" role="status">
          Loading…
        </p>
      ) : grants.length === 0 ? (
        <p className="muted">
          Nobody has a role on it yet{kind === "project" ? "; people with work given to them see it all the same" : ""}.
        </p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th scope="col">Who</th>
              <th scope="col">Role</th>
              <th scope="col">
                <span className="sr-only">Remove</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {grants.map((g) => (
              <tr key={g.userId}>
                <th scope="row" className="cell-title">
                  {g.name || g.username}
                </th>
                <td>
                  <select
                    className="select-small"
                    value={g.role}
                    aria-label={`Role of ${g.name || g.username}`}
                    onChange={(e) => void attempt(() => r.api!.setGrant({ userId: g.userId, kind, resource, role: e.target.value as Role }))}
                  >
                    {r.roles.map((x) => (
                      <option key={x.name} value={x.name}>
                        {roleText(x.name)}
                      </option>
                    ))}
                  </select>
                </td>
                <td className="cell-action">
                  <button type="button" className="btn btn-quiet btn-small" onClick={() => void attempt(() => r.api!.revokeGrant({ userId: g.userId, kind, resource }))}>
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <form className="role-add" onSubmit={give} aria-label={`Give a role on ${kind === "project" ? "the project" : resource}`}>
        <label>
          <span className="field-label">Who</span>
          <select value={userId} onChange={(e) => setUserId(e.target.value)}>
            <option value="">Choose someone</option>
            {r.people
              .filter((p) => !grants?.some((g) => g.userId === p.id))
              .map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name} ({p.username})
                </option>
              ))}
          </select>
        </label>
        <label>
          <span className="field-label">Role</span>
          <select value={role} onChange={(e) => setRole(e.target.value as Role)}>
            {r.roles.map((x) => (
              <option key={x.name} value={x.name}>
                {roleText(x.name)}
              </option>
            ))}
          </select>
        </label>
        <button type="submit" className="btn" disabled={!userId}>
          Give the role
        </button>
      </form>
    </section>
  );
}
