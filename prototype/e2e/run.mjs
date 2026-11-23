// Runs the browser tests against the production build: npm run e2e
import editor from "./editor.mjs";
import terminology from "./terminology.mjs";
import persistence from "./persistence.mjs";
import xliff from "./xliff.mjs";
import setup from "./setup.mjs";

let passed = 0;
let failed = 0;
for (const t of [editor, terminology, persistence, xliff, setup]) {
  const r = await t();
  passed += r.passed;
  failed += r.failed;
}
console.log(`\n${passed} passed, ${failed} failed`);
process.exit(failed ? 1 : 0);
