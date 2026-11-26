import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Relative base so the built page also works when served from a sub-path.
export default defineConfig({
  base: "./",
  // The sample Word document is bundled into the page (?inline).
  assetsInclude: ["**/*.docx"],
  plugins: [react()],
  // The interface translations are in ../i18n, shared with the desktop app.
  server: { fs: { allow: [".."] } },
});
