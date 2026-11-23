// A Trados SDLXLIFF file opens with its translations, states and locks,
// and is saved back with only the changed segments and states.
import { readFileSync, readdirSync } from "node:fs";
import { inflateRawSync } from "node:zlib";
import { run } from "./harness.mjs";

export default () =>
  run(
    "Bilingual XLIFF files",
    async ({
      send,
      evaluate,
      click,
      text,
      check,
      sleep,
      open,
      setFile,
      setValue,
      key,
      fixtures,
      downloads,
      waitFor,
    }) => {
      await open("/new");
      await setFile(
        "input[type=file]",
        `${fixtures}/Mietvertrag.docx.sdlxliff`,
      );
      check(
        await waitFor("document.body.innerText.includes('Trados SDLXLIFF')"),
        "file read as Trados SDLXLIFF",
      );
      let t = await text();
      check(
        t.includes("2 of 5 segments confirmed, 1 locked"),
        "summary of the file's segments: " +
          (t.match(/Trados SDLXLIFF[^\n]*/)?.[0] ?? ""),
      );
      check(
        t.includes("German (Germany)") &&
          t.includes("English (United Kingdom)") &&
          t.includes("From the file"),
        "languages from the file",
      );
      check(
        !(await evaluate("!!document.querySelector('#source')")),
        "languages cannot be changed",
      );
      await click("button", "Continue to terminology");
      await sleep(300);
      await click("button", "Continue to review");
      await sleep(300);
      await click("button", "Create project");
      check(
        await waitFor("document.querySelectorAll('.seg').length === 5"),
        "editor shows the 5 segments",
      );

      const row = (n) => `[...document.querySelectorAll('.seg')][${n - 1}]`;
      check(
        (await evaluate(`${row(1)}.className`)).includes("seg-confirmed") &&
          (await evaluate(`${row(1)}.innerText`)).includes("The tenant pays."),
        "translated segment arrives confirmed",
      );
      check(
        (await evaluate(`${row(3)}.className`)).includes("is-locked"),
        "locked segment marked",
      );
      check(
        (await evaluate(`${row(3)}.innerText`)).includes("Signed off"),
        "signed-off state shown",
      );
      check(
        (await evaluate(`${row(5)}.innerText`)).includes("Rejected") &&
          (await evaluate(`${row(5)}.className`)).includes("seg-draft"),
        "rejected segment is a draft to rework",
      );
      check(
        (await evaluate(`${row(2)}.className`)).includes("is-active"),
        "opens on the first open segment",
      );

      await key("a", "KeyA", 65, 2);
      await send("Input.insertText", { text: "The deposit" });
      await sleep(200);
      await key("Enter", "Enter", 13, 2);
      await sleep(300);
      check(
        (await evaluate(`${row(4)}.className`)).includes("is-active"),
        "confirming skips the locked segment",
      );

      await evaluate(`${row(3)}.querySelector('.seg-target-view').click()`);
      await sleep(300);
      t = await evaluate(`${row(3)}.innerText`);
      check(
        t.includes("Locked in the file") &&
          !(await evaluate(`!!${row(3)}.querySelector('.target-editor')`)),
        "locked segment opens read-only",
      );
      await key("Enter", "Enter", 13, 2);
      await sleep(200);
      check(
        (await evaluate("document.body.innerText")).includes(
          "This segment is locked in the file and cannot be changed.",
        ),
        "confirming a locked segment is refused",
      );
      check(
        !(await evaluate(
          "[...document.querySelectorAll('button')].some(b => b.textContent.includes('Preview'))",
        )),
        "no layout preview for XLIFF",
      );

      await click("button", "Save XLIFF");
      check(
        await waitFor(
          "document.body.innerText.includes('Saved Mietvertrag.docx.sdlxliff')",
        ),
        "saved under the file's own name",
      );
      await sleep(500);
      const files = readdirSync(downloads);
      check(
        files.includes("Mietvertrag.docx.sdlxliff"),
        "download: " + files.join(", "),
      );
      const out = readFileSync(
        `${downloads}/Mietvertrag.docx.sdlxliff`,
        "utf8",
      );
      const orig = readFileSync(
        `${fixtures}/Mietvertrag.docx.sdlxliff`,
        "utf8",
      );
      check(
        out.includes('<mrk mtype="seg" mid="2">The deposit</mrk>') &&
          out.includes(
            '<sdl:seg id="2" conf="Translated" origin="interactive"/>',
          ),
        "new translation and its Trados state written",
      );
      check(
        out.includes(
          '<sdl:seg id="1" conf="Translated" origin="tm" percent="100"/>',
        ) &&
          out.includes(
            '<sdl:seg id="3" locked="true" conf="ApprovedSignOff"/>',
          ) &&
          out.includes(
            '<sdl:seg id="5" conf="RejectedSignOff" origin="interactive"/>',
          ),
        "untouched segments keep their states",
      );
      check(
        orig
          .replace(
            '<mrk mtype="seg" mid="2"/>',
            '<mrk mtype="seg" mid="2">The deposit</mrk>',
          )
          .replace(
            '<sdl:seg id="2"/>',
            '<sdl:seg id="2" conf="Translated" origin="interactive"/>',
          ) === out,
        "nothing else in the file changed",
      );

      // A Trados project package: pick a file, translate, save a return package.
      await open("/new");
      await setFile(
        "input[type=file]",
        `${fixtures}/Mietvertrag Hofmann.sdlppx`,
      );
      check(
        await waitFor(
          "document.body.innerText.includes('from the Trados package')",
        ),
        "package read",
      );
      check(
        (await evaluate(
          "[...document.querySelectorAll('#pkg-file option')].map(o => o.value).join(',')",
        )) === "en-GB/Anlage.docx.sdlxliff,en-GB/Mietvertrag.docx.sdlxliff",
        "target files of the package offered",
      );
      await setValue("#pkg-file", "en-GB/Mietvertrag.docx.sdlxliff");
      check(
        await waitFor(
          "document.querySelector('#pkg-file')?.value === 'en-GB/Mietvertrag.docx.sdlxliff'",
        ),
        "chosen file read",
      );
      await click("button", "Continue to terminology");
      await sleep(300);
      await click("button", "Continue to review");
      await sleep(300);
      await click("button", "Create project");
      check(
        await waitFor("document.querySelectorAll('.seg').length === 5"),
        "package file opens in the editor",
      );
      await key("a", "KeyA", 65, 2);
      await send("Input.insertText", { text: "The deposit" });
      await sleep(200);
      await key("Enter", "Enter", 13, 2);
      await sleep(300);
      await click("button", "Save return package");
      check(
        await waitFor(
          "document.body.innerText.includes('Saved the return package Mietvertrag Hofmann.sdlrpx')",
        ),
        "return package saved",
      );
      await sleep(500);
      const zip = unzip(
        readFileSync(`${downloads}/Mietvertrag Hofmann.sdlrpx`),
      );
      check(
        Object.keys(zip).sort().join(",") ===
          "Mietvertrag Hofmann.sdlproj,de-DE/Mietvertrag.docx.sdlxliff,en-GB/Anlage.docx.sdlxliff,en-GB/Mietvertrag.docx.sdlxliff",
        "return package holds the project file and the bilingual files: " +
          Object.keys(zip).join(","),
      );
      check(
        zip["Mietvertrag Hofmann.sdlproj"].includes(
          'PackageType="ReturnPackage"',
        ),
        "project file marked as return package",
      );
      check(
        zip["en-GB/Mietvertrag.docx.sdlxliff"].includes(
          '<mrk mtype="seg" mid="2">The deposit</mrk>',
        ) && !zip["en-GB/Anlage.docx.sdlxliff"].includes("The deposit"),
        "only the translated file changed",
      );
    },
  );

// Reads the entries of a ZIP file (stored or deflated), enough for checks.
function unzip(buf) {
  const out = {};
  let eocd = buf.length - 22;
  while (eocd >= 0 && buf.readUInt32LE(eocd) !== 0x06054b50) eocd--;
  const count = buf.readUInt16LE(eocd + 10);
  let p = buf.readUInt32LE(eocd + 16);
  for (let i = 0; i < count; i++) {
    const method = buf.readUInt16LE(p + 10);
    const size = buf.readUInt32LE(p + 20);
    const nameLen = buf.readUInt16LE(p + 28);
    const extra = buf.readUInt16LE(p + 30);
    const comment = buf.readUInt16LE(p + 32);
    const local = buf.readUInt32LE(p + 42);
    const name = buf.subarray(p + 46, p + 46 + nameLen).toString("utf8");
    const data =
      local + 30 + buf.readUInt16LE(local + 26) + buf.readUInt16LE(local + 28);
    const raw = buf.subarray(data, data + size);
    out[name] = (method === 8 ? inflateRawSync(raw) : raw).toString("utf8");
    p += 46 + nameLen + extra + comment;
  }
  return out;
}
