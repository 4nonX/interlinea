import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { loadLanguage } from "./i18n";
// Light or dark before anything is drawn.
import "./theme";
import "./styles.css";

// The interface language's catalog first: the app's modules make some of
// their strings as they load.
void loadLanguage().then(async () => {
  const { App } = await import("./App");
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
});
