// The team server's API, as the web app uses it: signing in (password or
// identity provider), projects, files, segments, assignments, presence,
// memories and terminology, and the live feed of changes.

import type { CoreEntry } from "./termio";
import type { CoreRun } from "./editor/model";

/** An account, as the server shows it. */
export interface ServerUser {
  id: number;
  username: string;
  name: string;
  email?: string;
  admin: boolean;
  disabled: boolean;
  hasPassword: boolean;
  createdAt: string;
  version: number;
}

export interface ServerInfo {
  name: string;
  version: string;
  /** The identity provider people can sign in with. */
  provider?: string;
  /** The server has no accounts yet: the first administrator can be made. */
  setup: boolean;
}

/** Where a server is and who is signed in to it. */
export interface Connection {
  url: string;
  token: string;
  user: ServerUser;
  /** When the sign-in ends. */
  expiresAt: string;
}

export interface Problem {
  field: string;
  message: string;
}

/** An answer the server refused. */
export class ApiError extends Error {
  status: number;
  code: string;
  problems: Problem[];
  constructor(status: number, code: string, message: string, problems: Problem[] = []) {
    super(message);
    this.status = status;
    this.code = code;
    this.problems = problems;
  }
}

/** The server can't be reached: offline, wrong address, or it's down. */
export class OfflineError extends Error {}

/** A server address as typed: a scheme is added, the end trimmed. */
export function serverURL(typed: string): string {
  let s = typed.trim().replace(/\/+$/, "");
  if (s && !/^https?:\/\//i.test(s)) s = `https://${s}`;
  return s;
}

async function request(
  url: string,
  path: string,
  init: RequestInit & { token?: string; version?: number } = {},
): Promise<Response> {
  const headers = new Headers(init.headers);
  if (init.token) headers.set("Authorization", `Bearer ${init.token}`);
  if (init.version !== undefined) headers.set("If-Match", String(init.version));
  if (init.body !== undefined && typeof init.body === "string")
    headers.set("Content-Type", "application/json");
  let res: Response;
  try {
    res = await fetch(url + path, { ...init, headers });
  } catch (e) {
    throw new OfflineError(
      `The server at ${url} can't be reached${e instanceof Error && e.message ? ` (${e.message})` : ""}.`,
    );
  }
  if (!res.ok) {
    let code = "server";
    let message = `The server answered ${res.status}.`;
    let problems: Problem[] = [];
    try {
      const b = (await res.json()) as { code?: string; message?: string; problems?: Problem[] };
      code = b.code ?? code;
      message = b.message ?? message;
      problems = b.problems ?? [];
    } catch {
      // Not the server's JSON: a proxy's page, say.
    }
    throw new ApiError(res.status, code, message, problems);
  }
  return res;
}

async function json<T>(res: Response): Promise<T> {
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

/** What the server at an address is; fails if it isn't an Interlinea server. */
export async function serverInfo(url: string): Promise<ServerInfo> {
  const info = await json<ServerInfo>(await request(url, "/api/server"));
  if (info?.name !== "Interlinea") throw new ApiError(0, "notInterlinea", `${url} is not an Interlinea server.`);
  return info;
}

interface SigninResult {
  token: string;
  expiresAt: string;
  user: ServerUser;
}

const DEVICE = `Interlinea in ${navigator.userAgent.includes("Firefox") ? "Firefox" : navigator.userAgent.includes("Edg/") ? "Edge" : navigator.userAgent.includes("Chrome") ? "Chrome" : navigator.userAgent.includes("Safari") ? "Safari" : "a browser"}`;

function connection(url: string, r: SigninResult): Connection {
  return { url, token: r.token, user: r.user, expiresAt: r.expiresAt };
}

export async function signInWithPassword(url: string, username: string, password: string): Promise<Connection> {
  const r = await json<SigninResult>(
    await request(url, "/api/auth/password", {
      method: "POST",
      body: JSON.stringify({ username, password, device: DEVICE }),
    }),
  );
  return connection(url, r);
}

export async function setUpServer(
  url: string,
  a: { setupToken: string; username: string; name: string; email: string; password: string },
): Promise<Connection> {
  const r = await json<SigninResult>(
    await request(url, "/api/setup", { method: "POST", body: JSON.stringify({ ...a, device: DEVICE }) }),
  );
  return connection(url, r);
}

/** Where to send the browser to sign in through the provider; it comes back to returnTo. */
export async function providerSignInURL(url: string, returnTo: string): Promise<string> {
  const r = await json<{ url: string }>(
    await request(url, "/api/auth/oidc/start", {
      method: "POST",
      body: JSON.stringify({ return: returnTo, device: DEVICE }),
    }),
  );
  return r.url;
}

export async function exchangeCode(url: string, code: string): Promise<Connection> {
  const r = await json<SigninResult>(
    await request(url, "/api/auth/exchange", { method: "POST", body: JSON.stringify({ code }) }),
  );
  return connection(url, r);
}

/** What a provider sign-in left in the address's fragment, if anything. */
export function providerResult(hash: string): { code?: string; error?: string; linked?: string } | null {
  const h = hash.replace(/^#/, "");
  if (!/^interlinea-(code|error|linked)=/.test(h)) return null;
  const q = new URLSearchParams(h);
  return {
    code: q.get("interlinea-code") ?? undefined,
    error: q.get("interlinea-error") ?? undefined,
    linked: q.get("interlinea-linked") ?? undefined,
  };
}

// Types of the API's answers.

export type Role = "read" | "suggest" | "edit" | "approve" | "manage";

export interface Access {
  role: Role | "";
  assigned: boolean;
}

export interface CoreSegment {
  id: string;
  source: CoreRun[] | null;
  target: CoreRun[] | null;
  status: "new" | "draft" | "confirmed";
  locked?: boolean;
}

export interface StoredSegment extends CoreSegment {
  version: number;
  changedAt: string;
  changedBy: string;
}

export interface CoreMemorySetting {
  memory: string;
  lookup: boolean;
  concordance: boolean;
  update: boolean;
  penalty: number;
}

/** A project's settings as the core keeps them. */
export interface CoreSettings {
  name: string;
  client?: string;
  description?: string;
  due?: string;
  sourceLang: string;
  targetLangs: string[];
  files: { name: string; role: "translatable" | "reference"; targetLang?: string }[];
  memories: Record<string, CoreMemorySetting[]>;
  projectMemory: boolean;
  terminology: {
    mode: "layered" | "direct" | "own";
    masters: string[];
    writeTo?: string;
    addLanguages?: string[];
    location: "local" | "server";
    termbase?: string;
    autoUpdate: boolean;
  };
  analysis: { context: "previous" | "both" };
  qa: {
    disabled?: string[];
    enabled?: string[];
    rules?: unknown[];
    dictionaries?: Record<string, string>;
    words?: string[];
    uneditedFuzzy: boolean;
  };
  prepare: { pretranslate: boolean; minScore: number; confirmExact: boolean; confirmContext: boolean };
}

export interface ServerProject {
  id: number;
  settings: CoreSettings;
  version: number;
  createdAt: string;
  createdBy: string;
  changedAt: string;
  changedBy: string;
  access: Access;
}

export interface ServerProjectSummary extends ServerProject {
  segments: number;
  confirmed: number;
}

export type Task = "translate" | "review";

export interface Assignment {
  id: number;
  projectId: number;
  file?: string;
  targetLang?: string;
  fromSegment?: string;
  toSegment?: string;
  userId: number;
  userName: string;
  task: Task;
  due?: string;
  done: boolean;
  version: number;
  changedAt: string;
  changedBy: string;
}

export interface Presence {
  file: string;
  targetLang: string;
  segment: string;
  userId: number;
  userName: string;
  since: string;
  expires: string;
}

export interface Person {
  id: number;
  username: string;
  name: string;
}

export type Kind = "project" | "memory" | "termbase" | "all";

export interface Grant {
  userId: number;
  kind: Kind;
  /** A project's number, a memory's or termbase's name, or * for all of a kind. */
  resource: string;
  role: Role;
}

export interface GrantView extends Grant {
  username: string;
  name: string;
}

export interface SignIn {
  id: number;
  name: string;
  createdAt: string;
  lastUsed?: string;
  expiresAt: string;
}

export interface Identity {
  issuer: string;
  subject: string;
}

export interface CatalogMemory {
  name: string;
  sourceLang: string;
  targetLang: string;
  units: number;
  role?: Role;
}

export interface CatalogTermbase {
  name: string;
  languages: string[];
  entries: number;
  kind: "master" | "project";
  role?: Role;
}

export interface LayeredEntry extends CoreEntry {
  version: number;
  termbase: string;
}

export interface ProjectTermbase {
  name: string;
  kind: "master" | "project";
  upstreams?: string[];
  definition: { name?: string; languages: string[]; fields?: { name: string; level: string; kind: string; values?: string[] }[] };
  entries: LayeredEntry[];
}

export interface ProjectChange {
  seq: number;
  projectId?: number;
  project: string;
  kind: string;
  file?: string;
  targetLang?: string;
  segment?: string;
  version?: number;
  at: string;
  by: string;
}

/** A new file for a project: its original bytes and its segments per target language. */
export interface FileUpload {
  name: string;
  role: "translatable" | "reference";
  targetLang?: string;
  format: string;
  /** The original file, base64. */
  data: string;
  segments?: Record<string, CoreSegment[]>;
}

export function base64(bytes: Uint8Array): string {
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000)
    s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}

/** The API for a signed-in connection. */
export function client(conn: Connection) {
  const call = (path: string, init: RequestInit & { version?: number } = {}) =>
    request(conn.url, path, { ...init, token: conn.token });
  const get = async <T>(path: string) => json<T>(await call(path));
  const send = async <T>(method: string, path: string, body?: unknown, version?: number) =>
    json<T>(await call(path, { method, body: body === undefined ? undefined : JSON.stringify(body), version }));
  const p = (id: number) => `/api/projects/${id}`;
  const q = (o: Record<string, string>) => new URLSearchParams(o).toString();
  return {
    signOut: () => send<void>("POST", "/api/auth/signout"),
    me: () => get<{ user: ServerUser }>("/api/me"),
    people: () => get<Person[]>("/api/people"),
    catalog: () => get<{ memories: CatalogMemory[]; termbases: CatalogTermbase[] }>("/api/catalog"),
    projects: () => get<ServerProjectSummary[]>("/api/projects"),
    project: (id: number) => get<ServerProject>(p(id)),
    createProject: (body: {
      settings: CoreSettings;
      files: FileUpload[];
      memories?: { Name: string; SourceLang: string; TargetLang: string }[];
      termbase?: { name: string; languages?: string[] };
    }) => send<ServerProject>("POST", "/api/projects", body),
    updateSettings: (id: number, settings: CoreSettings, version: number) =>
      send<ServerProject>("PUT", `${p(id)}/settings`, settings, version),
    deleteProject: (id: number) => send<void>("DELETE", p(id)),
    fileBytes: async (id: number, name: string) =>
      new Uint8Array(await (await call(`${p(id)}/files/content?${q({ name })}`)).arrayBuffer()),
    segments: (id: number, file: string, lang: string) =>
      get<StoredSegment[]>(`${p(id)}/segments?${q({ file, lang })}`),
    saveSegment: (id: number, file: string, lang: string, segment: string, target: CoreRun[], status: string, version: number) =>
      send<StoredSegment>("PUT", `${p(id)}/segments?${q({ file, lang, segment })}`, { target, status }, version),
    memoryTMX: async (id: number, name: string) =>
      new Uint8Array(await (await call(`${p(id)}/memories/${encodeURIComponent(name)}/export`)).arrayBuffer()),
    terminology: (id: number) => get<ProjectTermbase[]>(`${p(id)}/terminology`),
    addTerm: (id: number, entry: CoreEntry) =>
      send<{ termbase: string; entry: LayeredEntry }>("POST", `${p(id)}/terms`, entry),
    assignments: (id: number) => get<Assignment[]>(`${p(id)}/assignments`),
    myAssignments: () => get<Assignment[]>("/api/me/assignments"),
    assign: (id: number, a: Partial<Assignment>) => send<Assignment>("POST", `${p(id)}/assignments`, a),
    updateAssignment: (id: number, a: Assignment) =>
      send<Assignment>("PUT", `${p(id)}/assignments/${a.id}`, a, a.version),
    unassign: (id: number, aid: number) => send<void>("DELETE", `${p(id)}/assignments/${aid}`),
    presence: (id: number) => get<Presence[]>(`${p(id)}/presence`),
    touch: (id: number, file: string, lang: string, segment: string) =>
      send<Presence>("PUT", `${p(id)}/presence`, { file, lang, segment }),
    leave: (id: number) => send<void>("DELETE", `${p(id)}/presence`),
    // Accounts (administrators) and roles (administrators, and managers of
    // the resource).
    users: () => get<ServerUser[]>("/api/users"),
    createUser: (u: { username: string; name: string; email: string; password: string; admin: boolean }) =>
      send<ServerUser>("POST", "/api/users", u),
    updateUser: (u: ServerUser, change: { name?: string; email?: string; admin?: boolean; disabled?: boolean }) =>
      send<ServerUser>("PATCH", `/api/users/${u.id}`, change, u.version),
    deleteUser: (id: number) => send<void>("DELETE", `/api/users/${id}`),
    setUserPassword: (id: number, password: string) => send<void>("PUT", `/api/users/${id}/password`, { password }),
    userTokens: (id: number) => get<SignIn[]>(`/api/users/${id}/tokens`),
    revokeUserTokens: (id: number) => send<void>("DELETE", `/api/users/${id}/tokens`),
    userIdentities: (id: number) => get<Identity[]>(`/api/users/${id}/identities`),
    linkIdentity: (id: number, issuer: string, subject: string) =>
      send<void>("POST", `/api/users/${id}/identities`, { issuer, subject }),
    unlinkIdentity: (id: number, issuer: string, subject: string) =>
      send<void>("DELETE", `/api/users/${id}/identities?${q({ issuer, subject })}`),
    userGrants: (id: number) => get<Grant[]>(`/api/users/${id}/grants`),
    grantsOn: (kind: Kind, resource: string) => get<GrantView[]>(`/api/grants?${q({ kind, resource })}`),
    setGrant: (g: Grant) => send<void>("PUT", "/api/grants", g),
    revokeGrant: (g: Pick<Grant, "userId" | "kind" | "resource">) =>
      send<void>("DELETE", `/api/grants?${q({ userId: String(g.userId), kind: g.kind, resource: g.resource })}`),
    ownTokens: () => get<(SignIn & { current: boolean })[]>("/api/me/tokens"),
    revokeOwnToken: (id: number) => send<void>("DELETE", `/api/me/tokens/${id}`),
    changePassword: (current: string, next: string) =>
      send<{ token: string; expiresAt: string; user: ServerUser }>("PUT", "/api/me/password", { current, new: next }),
    linkOwnIdentity: (returnTo: string) => send<{ url: string }>("POST", "/api/me/identities", { return: returnTo }),
    createMemory: (m: { name: string; sourceLang: string; targetLang: string }) =>
      send<CatalogMemory>("POST", "/api/memories", m),
    importTMX: async (name: string, tmx: Uint8Array) =>
      json<{ read: number; stored: number }>(
        await call(`/api/memories/${encodeURIComponent(name)}/import`, {
          method: "POST",
          body: tmx as unknown as BodyInit,
          headers: { "Content-Type": "application/xml" },
        }),
      ),
    createTermbase: (definition: { name: string; languages?: string[]; fields?: unknown[] }, upstreams?: string[]) =>
      send<unknown>("POST", "/api/termbases", { definition, upstreams }),
    setUpstreams: (name: string, upstreams: string[]) =>
      send<unknown>("PUT", `/api/termbases/${encodeURIComponent(name)}/upstreams`, { upstreams }),
    addFile: (id: number, f: FileUpload, version: number) => send<ServerProject>("POST", `${p(id)}/files`, f, version),
    removeFile: (id: number, name: string, version: number) =>
      send<ServerProject>("DELETE", `${p(id)}/files?${q({ name })}`, undefined, version),
    pretranslate: (id: number, prepare?: CoreSettings["prepare"], lang?: string) =>
      send<{ lang: string; filled: number }[]>("POST", `${p(id)}/pretranslate`, { lang: lang ?? "", prepare }),
    importTermbase: async (name: string, tbx: Uint8Array) =>
      json<{ added: number }>(
        await call(`/api/termbases/${encodeURIComponent(name)}/import?${q({ format: "tbx", match: "none" })}`, {
          method: "POST",
          body: tbx as unknown as BodyInit,
          headers: { "Content-Type": "application/xml" },
        }),
      ),
  };
}

export type Client = ReturnType<typeof client>;

/** An event of the live feed. */
export type FeedEvent =
  | { type: "project"; change: ProjectChange }
  | { type: "memory"; memory: string }
  | { type: "termbase"; termbase: string }
  | { type: "account" }
  | { type: "presence"; projectId: number }
  | { type: "reset" }
  | { type: "signedOut" };

/**
 * Follows the live feed until stopped, reconnecting after a dropped
 * connection with the last event seen so nothing is missed. Returns the
 * function that stops it.
 */
export function follow(conn: Connection, on: (e: FeedEvent) => void, onState?: (live: boolean) => void): () => void {
  let stopped = false;
  let ctrl: AbortController | null = null;
  let last = "";
  let wait = 1000;
  const loop = async () => {
    while (!stopped) {
      ctrl = new AbortController();
      try {
        const headers: Record<string, string> = { Authorization: `Bearer ${conn.token}` };
        if (last) headers["Last-Event-ID"] = last;
        const res = await fetch(`${conn.url}/api/events`, { headers, signal: ctrl.signal });
        if (res.status === 401) {
          on({ type: "signedOut" });
          return;
        }
        if (!res.ok || !res.body) throw new Error(String(res.status));
        onState?.(true);
        wait = 1000;
        const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
        let buf = "";
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          buf += value;
          let cut;
          while ((cut = buf.indexOf("\n\n")) >= 0) {
            const block = buf.slice(0, cut);
            buf = buf.slice(cut + 2);
            let type = "";
            let data = "";
            let id = "";
            for (const line of block.split("\n")) {
              if (line.startsWith("event: ")) type = line.slice(7);
              else if (line.startsWith("data: ")) data += line.slice(6);
              else if (line.startsWith("id: ")) id = line.slice(4);
            }
            if (id) last = id;
            if (!type) continue;
            const d = data ? (JSON.parse(data) as Record<string, unknown>) : {};
            switch (type) {
              case "project":
                on({ type, change: d as unknown as ProjectChange });
                break;
              case "memory":
                on({ type, memory: String(d.memory ?? "") });
                break;
              case "termbase":
                on({ type, termbase: String(d.termbase ?? "") });
                break;
              case "account":
                on({ type });
                break;
              case "presence":
                on({ type, projectId: Number(d.projectId) });
                break;
              case "reset":
                on({ type });
                break;
              case "signedOut":
                on({ type });
                return;
            }
          }
        }
      } catch {
        // Dropped or refused: try again shortly.
      }
      if (stopped) return;
      onState?.(false);
      await new Promise((r) => setTimeout(r, wait));
      wait = Math.min(wait * 2, 30000);
    }
  };
  void loop();
  return () => {
    stopped = true;
    ctrl?.abort();
  };
}
