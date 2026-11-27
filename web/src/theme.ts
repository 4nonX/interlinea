// Light or dark. The settings choose one, or follow the system (the
// default); the choice is kept in this browser. The whole look follows
// <html data-theme>, which styles.css reads.

import { useSyncExternalStore } from "react";

export type Appearance = "" | "light" | "dark";

const KEY = "interlinea.appearance";

const listeners = new Set<() => void>();
const system = window.matchMedia("(prefers-color-scheme: dark)");

/** The choice in the settings; "" follows the system. */
export function chosenAppearance(): Appearance {
  try {
    const v = localStorage.getItem(KEY);
    return v === "light" || v === "dark" ? v : "";
  } catch {
    return "";
  }
}

let choice: Appearance = chosenAppearance();

/** Light or dark as the interface is shown now. */
export function shownAppearance(): "light" | "dark" {
  return choice || (system.matches ? "dark" : "light");
}

function show() {
  const shown = shownAppearance();
  document.documentElement.dataset.theme = shown;
  for (const l of listeners) l();
}

/** Keeps the choice and shows it at once. */
export function chooseAppearance(a: Appearance) {
  choice = a;
  try {
    if (a) localStorage.setItem(KEY, a);
    else localStorage.removeItem(KEY);
  } catch {
    // Without storage the choice lasts until the page is closed.
  }
  show();
}

/** The choice and what is shown, for components that show them. */
export function useAppearance(): { chosen: Appearance; shown: "light" | "dark" } {
  const shown = useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    shownAppearance,
  );
  return { chosen: choice, shown };
}

// The system changes from light to dark: followed when it is the choice.
system.addEventListener("change", () => {
  if (!choice) show();
});

// Another tab changed the choice.
window.addEventListener("storage", (e) => {
  if (e.key !== KEY) return;
  choice = chosenAppearance();
  show();
});

show();
