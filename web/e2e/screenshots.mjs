// Takes the screenshots of the browser client used in the docs:
//   npm run build && npm run screenshots
import { writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { start } from "./harness.mjs";

const dir = resolve(process.argv[2] ?? "../docs/screenshots");
const h = await start();
const { send, open, waitFor, sleep, click, finish } = h;

const shot = async (name) => {
  const { data } = await send("Page.captureScreenshot", { format: "png" });
  writeFileSync(join(dir, name), Buffer.from(data, "base64"));
  console.log("saved", join(dir, name));
};

try {
  await send("Emulation.setDeviceMetricsOverride", {
    width: 1440,
    height: 900,
    deviceScaleFactor: 1,
    mobile: false,
  });

  // The editor on the sample lease, on the segment it opens on, with a
  // recognized term beside it. A fresh load starts on Projects while the
  // sample data is set up, so the route is set once the page is there.
  await open("/projects", { wait: 2500 });
  await open("/editor/p-hofmann", { reload: false, wait: 500 });
  if (!(await waitFor("!!document.querySelector('.seg.is-active .term-hit')", 20000)))
    throw new Error("the editor did not open");
  await sleep(1500);
  await shot("editor.png");

  // Terminology, with the first entry of the sample termbase open.
  await open("/terminology", { reload: false, wait: 500 });
  if (!(await waitFor("document.querySelector('main')?.innerText.length > 200", 15000)))
    throw new Error("terminology did not open");
  await sleep(1500);
  await shot("terminology.png");

  // A glossary of the termbase, at the step that lays it out.
  await click(".pane-tools button", "Export");
  await sleep(300);
  await click("dialog label.choice", ".pdf");
  await click("dialog button", "Continue");
  await sleep(200);
  await click("dialog button", "Continue");
  await sleep(600);
  await shot("glossary-export.png");
  await click("dialog button", "Cancel");
  await sleep(300);

  // The review of what the project termbase changed, before it goes into
  // the master.
  await open("/merge/p-hofmann", { reload: false, wait: 500 });
  if (!(await waitFor("document.querySelector('main')?.innerText.length > 200", 15000)))
    throw new Error("the merge review did not open");
  await sleep(1500);
  await shot("merge-review.png");

  // The projects, and the memories with their units.
  await open("/projects", { reload: false, wait: 500 });
  await sleep(1500);
  await shot("projects.png");
  await open("/memories", { reload: false, wait: 500 });
  await sleep(1500);
  await shot("memories.png");
} finally {
  finish();
}
