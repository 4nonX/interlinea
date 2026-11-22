import { useEffect, useState } from "react";
import { BookText, FolderOpen, GitBranch, Settings as SettingsIcon } from "lucide-react";
import { StoreProvider, useStore } from "./store";
import { Toast } from "./components/ui";
import { Projects } from "./screens/Projects";
import { NewProject } from "./screens/NewProject";
import { Terminology } from "./screens/Terminology";
import { MergeReview } from "./screens/MergeReview";
import { Settings } from "./screens/Settings";
import { Editor } from "./screens/Editor";

export type Route =
  | { page: "projects" }
  | { page: "new"; sample?: number }
  | { page: "terminology"; termbaseId?: string; entryId?: string }
  | { page: "merge"; projectId: string }
  | { page: "editor"; projectId: string }
  | { page: "settings" };

export function parseRoute(hash: string): Route {
  const [page, a, b] = hash.replace(/^#\/?/, "").split("/");
  switch (page) {
    case "new":
      return a === "sample" && b !== undefined ? { page: "new", sample: Number(b) } : { page: "new" };
    case "terminology":
      return { page: "terminology", termbaseId: a || undefined, entryId: b || undefined };
    case "merge":
      return a ? { page: "merge", projectId: a } : { page: "projects" };
    case "editor":
      return a ? { page: "editor", projectId: a } : { page: "projects" };
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
      return r.sample !== undefined ? `#/new/sample/${r.sample}` : "#/new";
    case "terminology":
      return `#/terminology${r.termbaseId ? `/${r.termbaseId}` : ""}${r.entryId ? `/${r.entryId}` : ""}`;
    case "merge":
      return `#/merge/${r.projectId}`;
    case "editor":
      return `#/editor/${r.projectId}`;
    case "settings":
      return "#/settings";
  }
}

// Routing works in memory first and mirrors the route into the URL hash
// where the host allows it, so the prototype also runs in sandboxed viewers
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
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
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
  const active = route.page === "new" || route.page === "merge" || route.page === "editor" ? "projects" : route.page;
  const items = [
    { id: "projects", label: "Projects", icon: FolderOpen, to: { page: "projects" } as Route },
    { id: "terminology", label: "Terminology", icon: BookText, to: { page: "terminology" } as Route },
    { id: "settings", label: "Settings", icon: SettingsIcon, to: { page: "settings" } as Route },
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
            <a href={href(to)} aria-current={active === id ? "page" : undefined}>
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
        <span className="rail-note">Prototype with sample data</span>
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
      // A new sample link starts a fresh flow rather than reusing the open one.
      return <NewProject key={route.sample ?? "file"} sample={route.sample} />;
    case "terminology":
      return <Terminology termbaseId={route.termbaseId} entryId={route.entryId} />;
    case "merge":
      return <MergeReview projectId={route.projectId} />;
    case "editor":
      return <Editor key={route.projectId} projectId={route.projectId} />;
    case "settings":
      return <Settings />;
  }
}

function Shell() {
  const route = useRoute();
  return (
    <div className="app">
      <a className="skip-link" href="#main">
        Skip to content
      </a>
      <Rail route={route} />
      <main id="main" className={route.page === "editor" ? "main main-editor" : "main"}>
        <Screen route={route} />
      </main>
      <Toast />
    </div>
  );
}

export function App() {
  return (
    <StoreProvider>
      <Shell />
    </StoreProvider>
  );
}
