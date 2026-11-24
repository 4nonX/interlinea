import { useEffect, useState } from "react";
import {
  BookText,
  Database,
  FolderOpen,
  GitBranch,
  Settings as SettingsIcon,
  Users,
} from "lucide-react";
import { StoreProvider, useStore } from "./store";
import { RemoteProvider, errorText, serverNumber, useRemote } from "./remote";
import { Toast } from "./components/ui";
import { Projects } from "./screens/Projects";
import { NewProject } from "./screens/NewProject";
import { Terminology } from "./screens/Terminology";
import { MergeReview } from "./screens/MergeReview";
import { Settings } from "./screens/Settings";
import { Editor } from "./screens/Editor";
import { ProjectSettings } from "./screens/ProjectSettings";
import { Memories } from "./screens/Memories";
import { ServerProjectPage } from "./screens/ServerProjectPage";
import { Accounts } from "./screens/Accounts";
import { ServerMergeReview } from "./screens/ServerMergeReview";

export type Route =
  | { page: "projects" }
  | { page: "new" }
  | { page: "terminology"; termbaseId?: string; entryId?: string }
  | { page: "merge"; projectId: string }
  | { page: "editor"; projectId: string; fileId?: string; lang?: string }
  | { page: "project"; projectId: string }
  | { page: "memories" }
  | { page: "accounts" }
  | { page: "review"; id: number }
  | { page: "settings" };

export function parseRoute(hash: string): Route {
  const [page, a, b, c] = hash
    .replace(/^#\/?/, "")
    .split("/")
    .map((x) => {
      try {
        return decodeURIComponent(x);
      } catch {
        return x;
      }
    });
  switch (page) {
    case "new":
      return { page: "new" };
    case "terminology":
      return {
        page: "terminology",
        termbaseId: a || undefined,
        entryId: b || undefined,
      };
    case "merge":
      return a ? { page: "merge", projectId: a } : { page: "projects" };
    case "editor":
      return a
        ? {
            page: "editor",
            projectId: a,
            fileId: b || undefined,
            lang: c || undefined,
          }
        : { page: "projects" };
    case "project":
      return a ? { page: "project", projectId: a } : { page: "projects" };
    case "memories":
      return { page: "memories" };
    case "accounts":
      return { page: "accounts" };
    case "review":
      return a && /^\d+$/.test(a) ? { page: "review", id: Number(a) } : { page: "terminology" };
    case "settings":
      return { page: "settings" };
    default:
      return { page: "projects" };
  }
}

export function href(r: Route): string {
  switch (r.page) {
    case "projects":
      return "#/projects";
    case "new":
      return "#/new";
    case "terminology":
      return `#/terminology${r.termbaseId ? `/${r.termbaseId}` : ""}${r.entryId ? `/${r.entryId}` : ""}`;
    case "merge":
      return `#/merge/${r.projectId}`;
    case "editor":
      return `#/editor/${r.projectId}${r.fileId ? `/${encodeURIComponent(r.fileId)}${r.lang ? `/${r.lang}` : ""}` : ""}`;
    case "project":
      return `#/project/${r.projectId}`;
    case "memories":
      return "#/memories";
    case "accounts":
      return "#/accounts";
    case "review":
      return `#/review/${r.id}`;
    case "settings":
      return "#/settings";
  }
}

// Routing works in memory first and mirrors the route into the URL hash
// where the host allows it, so the page also runs in sandboxed viewers
// that drop or rewrite hash paths.
const ROUTE_EVENT = "interlinea:route";
let current: Route = parseRoute(window.location.hash);

export function go(r: Route) {
  current = r;
  window.dispatchEvent(new Event(ROUTE_EVENT));
  try {
    const h = href(r);
    if (window.location.hash !== h) history.pushState(null, "", h);
  } catch {
    // The host does not allow changing the address; in-memory routing suffices.
  }
}

function useRoute(): Route {
  const [route, setRoute] = useState(current);
  useEffect(() => {
    const onInternal = () => setRoute(current);
    const onHash = () => {
      current = parseRoute(window.location.hash);
      setRoute(current);
    };
    window.addEventListener(ROUTE_EVENT, onInternal);
    window.addEventListener("hashchange", onHash);
    window.addEventListener("popstate", onHash);
    // Plain links inside the app navigate through go().
    const onClick = (e: MouseEvent) => {
      if (
        e.defaultPrevented ||
        e.button !== 0 ||
        e.metaKey ||
        e.ctrlKey ||
        e.shiftKey ||
        e.altKey
      )
        return;
      const a = (e.target as Element | null)?.closest?.("a[href^='#/']");
      if (!a) return;
      e.preventDefault();
      go(parseRoute(a.getAttribute("href")!));
    };
    document.addEventListener("click", onClick);
    return () => {
      window.removeEventListener(ROUTE_EVENT, onInternal);
      window.removeEventListener("hashchange", onHash);
      window.removeEventListener("popstate", onHash);
      document.removeEventListener("click", onClick);
    };
  }, []);
  return route;
}

function Rail({ route }: { route: Route }) {
  const { state } = useStore();
  const remote = useRemote();
  const active =
    route.page === "new" ||
    route.page === "merge" ||
    route.page === "editor" ||
    route.page === "project"
      ? "projects"
      : route.page === "review"
        ? "terminology"
        : route.page;
  const items = [
    {
      id: "projects",
      label: "Projects",
      icon: FolderOpen,
      to: { page: "projects" } as Route,
    },
    {
      id: "memories",
      label: "Memories",
      icon: Database,
      to: { page: "memories" } as Route,
    },
    {
      id: "terminology",
      label: "Terminology",
      icon: BookText,
      to: { page: "terminology" } as Route,
    },
    ...(remote.conn?.user.admin
      ? [
          {
            id: "accounts",
            label: "Accounts",
            icon: Users,
            to: { page: "accounts" } as Route,
          },
        ]
      : []),
    {
      id: "settings",
      label: "Settings",
      icon: SettingsIcon,
      to: { page: "settings" } as Route,
    },
  ];
  return (
    <nav className="rail" aria-label="Main">
      <a className="brand" href="#/projects">
        <span className="brand-mark" aria-hidden="true">
          <svg viewBox="0 0 20 20" width="20" height="20">
            <rect x="2" y="4" width="16" height="2" rx="1" />
            <rect x="2" y="9" width="11" height="2" rx="1" opacity=".55" />
            <rect x="2" y="14" width="16" height="2" rx="1" />
          </svg>
        </span>
        Interlinea
      </a>
      <ul>
        {items.map(({ id, label, icon: Icon, to }) => (
          <li key={id}>
            <a
              href={href(to)}
              aria-current={active === id ? "page" : undefined}
            >
              <Icon size={17} aria-hidden="true" />
              {label}
            </a>
          </li>
        ))}
      </ul>
      <div className="rail-foot">
        {state.gitWords && (
          <span className="rail-mode">
            <GitBranch size={14} aria-hidden="true" />
            Git terms on
          </span>
        )}
      </div>
    </nav>
  );
}

function Screen({ route }: { route: Route }) {
  // Move focus to the page heading on navigation, so keyboard and screen
  // reader users land on the new content. The editor puts the caret in the
  // active segment instead; its label and the status line say where you are.
  useEffect(() => {
    if (route.page === "editor") return;
    document.getElementById("page-title")?.focus({ preventScroll: true });
  }, [route]);
  switch (route.page) {
    case "projects":
      return <Projects />;
    case "new":
      return <NewProject />;
    case "terminology":
      return (
        <Terminology termbaseId={route.termbaseId} entryId={route.entryId} />
      );
    case "merge":
      return <MergeReview projectId={route.projectId} />;
    case "editor":
      return (
        <ServerProject projectId={route.projectId}>
          <Editor
            key={`${route.projectId}/${route.fileId ?? ""}/${route.lang ?? ""}`}
            projectId={route.projectId}
            fileId={route.fileId}
            lang={route.lang}
          />
        </ServerProject>
      );
    case "project":
      return (
        <ServerProject projectId={route.projectId}>
          {serverNumber(route.projectId) === null ? (
            <ProjectSettings projectId={route.projectId} />
          ) : (
            <ServerProjectPage projectId={route.projectId} />
          )}
        </ServerProject>
      );
    case "memories":
      return <Memories />;
    case "accounts":
      return <Accounts />;
    case "review":
      return <ServerMergeReview id={route.id} />;
    case "settings":
      return <Settings />;
  }
}

/**
 * A project on the team server is opened from there before its page
 * shows; a project on this computer shows at once.
 */
function ServerProject({ projectId, children }: { projectId: string; children: React.ReactNode }) {
  const { state } = useStore();
  const r = useRemote();
  const n = serverNumber(projectId);
  const loaded = state.projects.some((p) => p.id === projectId);
  const [problem, setProblem] = useState("");
  useEffect(() => {
    if (n === null || !r.conn) return;
    let live = true;
    setProblem("");
    // Opened fresh each time, so the translations are the server's latest.
    r.open(n).catch((e: unknown) => live && setProblem(errorText(e)));
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [n, r.conn]);
  if (n === null) return <>{children}</>;
  if (!r.conn)
    return (
      <div className="page page-narrow">
        <div className="empty">
          <p className="empty-title">This project is on a team server</p>
          <p>
            Sign in to the server in <a href="#/settings">Settings</a> to open it.
          </p>
        </div>
      </div>
    );
  if (problem)
    return (
      <div className="page page-narrow">
        <div className="empty">
          <p className="empty-title">The project could not be opened</p>
          <p className="error" role="alert">
            {problem}
          </p>
          <p>
            <a href="#/projects">Back to the projects</a>
          </p>
        </div>
      </div>
    );
  if (!loaded)
    return (
      <p className="boot" role="status">
        Opening the project from {new URL(r.conn.url).host}…
      </p>
    );
  return <>{children}</>;
}

function Shell() {
  const route = useRoute();
  return (
    <div className="app">
      <a className="skip-link" href="#main">
        Skip to content
      </a>
      <Rail route={route} />
      <main
        id="main"
        className={route.page === "editor" ? "main main-editor" : "main"}
      >
        <Screen route={route} />
      </main>
      <Toast />
    </div>
  );
}

export function App() {
  return (
    <StoreProvider>
      <RemoteProvider>
        <Shell />
      </RemoteProvider>
    </StoreProvider>
  );
}
