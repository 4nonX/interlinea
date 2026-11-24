// Runs the browser tests against the production build: npm run e2e
import editor from "./editor.mjs";
import terminology from "./terminology.mjs";
import persistence from "./persistence.mjs";
import xliff from "./xliff.mjs";
import setup from "./setup.mjs";
import server from "./server.mjs";

let passed = 0;
let failed = 0;
// The team server test needs a built interlinea-server ($INTERLINEA_SERVER);
// CI always has one.
const tests = [editor, terminology, persistence, xliff, setup];
if (process.env.INTERLINEA_SERVER) tests.push(server);
else if (process.env.CI) {
  console.log("FAIL INTERLINEA_SERVER is not set, so the team server test can't run");
  failed++;
} else console.log("(team server test skipped: set INTERLINEA_SERVER to a built interlinea-server)");
for (const t of tests) {
  const r = await t();
  passed += r.passed;
  failed += r.failed;
}
console.log(`\n${passed} passed, ${failed} failed`);
process.exit(failed ? 1 : 0);
