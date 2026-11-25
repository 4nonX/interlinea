// A Trados SDLXLIFF file opens with its translations, states and locks,
// and is saved back with only the changed segments and states.
import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
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
      await click("button", "Continue to files and languages");
      await sleep(300);
      await setFile(
        "input[type=file]",
        `${fixtures}/Mietvertrag.docx.sdlxliff`,
      );
      check(
        await waitFor("document.body.innerText.includes('SDLXLIFF')"),
        "file read as Trados SDLXLIFF",
      );
      let t = await text();
      check(
        t.includes("2 of 5 segments confirmed, 1 locked"),
        "summary of the file's segments: " +
          (t.match(/SDLXLIFF[^\n]*/)?.[0] ?? ""),
      );
      check(
        t.includes("German (Germany)") &&
          t.includes("English (United Kingdom)") &&
          t.includes("From the bilingual files"),
        "languages from the file",
      );
      check(
        !(await evaluate("!!document.querySelector('#source')")) &&
          (await evaluate(
            "[...document.querySelectorAll('.target-list input')].find(i => i.closest('label').textContent.includes('English (United Kingdom)')).disabled",
          )),
        "languages cannot be changed",
      );
      await click("button", "Continue to memories");
      await sleep(300);
      await click("button", "Continue to terminology");
      await sleep(300);
      await click("button", "Continue to review");
      await waitFor("!!document.querySelector('.analysis')");
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

      // Trados keeps its own record of each segment: they are not split or joined.
      await evaluate("(document.querySelector('.seg .seg-target-view') ?? document.querySelector('.seg .target-editor')).click()");
      await sleep(400);
      check(
        await evaluate("[...document.querySelectorAll('.tool-row button')].filter(b => /Split|Join with next/.test(b.textContent)).length === 2 && [...document.querySelectorAll('.tool-row button')].filter(b => /Split|Join with next/.test(b.textContent)).every(b => b.disabled)") &&
          (await text()).includes("keeps its own record of every segment"),
        "a Trados file's segments cannot be split or joined, and the editor says why",
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

      // A Trados project package: all its files in one project, one return package.
      await open("/new");
      await click("button", "Continue to files and languages");
      await sleep(300);
      await setFile(
        "input[type=file]",
        `${fixtures}/Mietvertrag Hofmann.sdlppx`,
      );
      check(
        await waitFor(
          "document.body.innerText.includes('From the project package')",
        ),
        "package read",
      );
      check(
        (await evaluate(
          "[...document.querySelectorAll('.file-table .cell-title')].map(e => e.textContent).join(',')",
        )) === "en-GB/Anlage.docx.sdlxliff,en-GB/Mietvertrag.docx.sdlxliff",
        "every target file of the package is a file of the project",
      );
      check(
        (await evaluate("document.getElementById('pname').value")) ===
          "Mietvertrag Hofmann",
        "project named after the package",
      );
      check(
        (await text()).includes("A project with this name exists already."),
        "the name of the sample project is taken",
      );
      await setValue("#pname", "Mietvertrag Hofmann Rücklauf");
      await sleep(300);
      await click("button", "Continue to memories");
      await sleep(300);
      await click("button", "Continue to terminology");
      await sleep(300);
      await click("button", "Continue to review");
      await waitFor("!!document.querySelector('.analysis')");
      t = await text();
      check(
        t.includes("2 files to translate") &&
          t.includes("from the project package Mietvertrag Hofmann.sdlppx"),
        "review counts the package's files",
      );
      await click("button", "Create project");
      check(
        await waitFor("!!document.querySelector('#ed-file')"),
        "editor offers the package's files",
      );
      await setValue(
        "#ed-file",
        await evaluate(
          "[...document.querySelectorAll('#ed-file option')].find(o => o.textContent.includes('Mietvertrag')).value",
        ),
      );
      check(
        await waitFor(
          "document.querySelectorAll('.seg').length === 5 && document.querySelector('#ed-file option:checked')?.textContent.includes('Mietvertrag')",
        ),
        "the package's Mietvertrag file opens",
      );
      await evaluate(
        "document.querySelector('.seg.is-active .target-editor')?.focus()",
      );
      await sleep(200);
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

      // A plain XLIFF file is cut like a document: split anywhere, joined
      // within a unit of the file, and written as the file has its units.
      await open("/new", { keep: true, wait: 0 });
      await waitFor("[...document.querySelectorAll('button')].some(b => b.textContent.includes('Continue to files and languages'))", 15000);
      await click("button", "Continue to files and languages");
      await sleep(300);
      await setFile("input[type=file]", `${fixtures}/Hinweise.xlf`);
      await waitFor("document.body.innerText.includes('XLIFF 1.2')", 15000);
      await setValue("#pname", "Freie Segmente");
      for (const b of ["Continue to memories", "Continue to terminology", "Continue to review"]) {
        await click("button", b);
        await sleep(300);
      }
      await waitFor("!!document.querySelector('.analysis')");
      await click("button", "Create project");
      await waitFor("document.querySelectorAll('.seg').length === 2", 15000);
      await evaluate("(document.querySelector('.seg .seg-target-view') ?? document.querySelector('.seg .target-editor')).click()");
      await sleep(400);
      const cutButton = (label) => `[...document.querySelectorAll('.tool-row button')].find(b => b.textContent.includes(${JSON.stringify(label)}))`;
      check(await evaluate(`!${cutButton("Split")}.disabled && ${cutButton("Join with next")}.disabled`) && !(await text()).includes("keeps its own record"), "a plain XLIFF segment can be split; the next one is another unit of the file");
      await evaluate(`${cutButton("Split")}.click()`);
      await waitFor("!!document.querySelector('.split-source')", 5000);
      await evaluate("(() => { const el = document.querySelector('.split-source'); const n = el.firstChild; const r = document.createRange(); r.setStart(n, n.textContent.indexOf('monatlich')); r.collapse(true); const s = getSelection(); s.removeAllRanges(); s.addRange(r); })()");
      await waitFor("![...document.querySelectorAll('dialog[open] button')].find(b => b.textContent.trim() === 'Split').disabled", 3000);
      await evaluate("[...document.querySelectorAll('dialog[open] button')].find(b => b.textContent.trim() === 'Split').click()");
      check(await waitFor("document.querySelectorAll('.seg').length === 3", 5000), "split in two parts");
      await sleep(300);
      await evaluate("[...document.querySelectorAll('.seg')][1].querySelector('.seg-target-view, .target-editor').click()");
      await waitFor("document.querySelectorAll('.seg')[1].classList.contains('is-active') && !!document.querySelector('.seg.is-active .target-editor')", 5000);
      await evaluate("document.querySelector('.seg.is-active .target-editor').focus()");
      await send("Input.insertText", { text: "every month." });
      await key("Enter", "Enter", 13, 2);
      await sleep(400);
      await click("button", "Save XLIFF");
      check(await waitFor("document.body.innerText.includes('Saved Hinweise.xlf')"), "the XLIFF file is saved");
      await sleep(500);
      const cutOut = readFileSync(`${downloads}/Hinweise.xlf`, "utf8");
      check(/<target state="[^"]*">The tenant pays monthly\. every month\.<\/target>/.test(cutOut) && cutOut.includes("<source>Der Mieter zahlt monatlich.</source>"), "the unit has the parts' translations one after the other, its source as it was: " + (cutOut.match(/<target[^>]*>[^<]*<\/target>/)?.[0] ?? cutOut));
      await evaluate("[...document.querySelectorAll('.seg')][0].querySelector('.seg-target-view, .target-editor').click()");
      await waitFor("document.querySelectorAll('.seg')[0].classList.contains('is-active')", 5000);
      await sleep(300);
      check(await evaluate(`!${cutButton("Join with next")}.disabled`), "the parts can be joined again");
      await evaluate(`${cutButton("Join with next")}.click()`);
      check(await waitFor("document.querySelectorAll('.seg').length === 2", 5000), "joined, the unit is one segment again");

      // A TagEditor file never segmented: it names no target language, so
      // it is for the project's; its sentences are the segments, and each
      // one translated is written back as a segment of its own.
      await open("/new", { keep: true, wait: 0 });
      await waitFor("[...document.querySelectorAll('button')].some(b => b.textContent.includes('Continue to files and languages'))", 15000);
      await click("button", "Continue to files and languages");
      await sleep(300);
      await setFile("input[type=file]", resolve(fixtures, "../../../internal/realworld/testdata/tageditor/okapi-issue-164.doc.ttx"));
      check(await waitFor("document.body.innerText.includes('TagEditor TTX')", 15000), "file read as a TagEditor file");
      await evaluate("[...document.querySelectorAll('.target-list input')].forEach(i => { const want = i.closest('label').textContent.includes('German (Germany)'); if (i.checked !== want) i.click(); })");
      await sleep(300);
      await setValue("#pname", "TagEditor");
      for (const b of ["Continue to memories", "Continue to terminology", "Continue to review"]) {
        await click("button", b);
        await sleep(300);
      }
      await waitFor("!!document.querySelector('.analysis')");
      await click("button", "Create project");
      check(await waitFor("document.querySelectorAll('.seg').length === 12", 15000), "the file's 12 sentences are the segments");
      await evaluate("(document.querySelector('.seg .seg-target-view') ?? document.querySelector('.seg .target-editor')).click()");
      await waitFor("!!document.querySelector('.seg.is-active .target-editor')", 5000);
      await evaluate("document.querySelector('.seg.is-active .target-editor').focus()");
      await send("Input.insertText", { text: "Erster Satz." });
      await key("Enter", "Enter", 13, 2);
      await sleep(400);
      await click("button", "Save XLIFF");
      check(await waitFor("document.body.innerText.includes('Saved okapi-issue-164.doc.ttx')"), "the TagEditor file is saved under its own name");
      await sleep(500);
      const ttx = readFileSync(`${downloads}/okapi-issue-164.doc.ttx`);
      const ttxText = ttx.subarray(2).toString("utf16le");
      check(
        ttx[0] === 0xff && ttx[1] === 0xfe && ttxText.includes('TargetLanguage="DE-DE"') && /<Tu Origin="manual" MatchPercent="0"><Tuv Lang="EN-US">[^<]+<\/Tuv><Tuv Lang="DE-DE">Erster Satz\.<\/Tuv><\/Tu>/.test(ttxText),
        "written in UTF-16 as TagEditor writes, naming the language, the sentence a segment: " + (ttxText.match(/<Tu .{0,200}/)?.[0] ?? ttxText.slice(0, 300)),
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
