// Project setup: several files and target languages, reference files,
// memories with penalties and a new memory, explicit master languages,
// pre-translation, templates, the project's settings page, the memories
// page, deleting a project, and work saved by the first version moved
// into the current shape.
import { run } from "./harness.mjs";

export default () =>
  run(
    "Project setup and settings",
    async ({
      send,
      evaluate,
      click,
      text,
      body,
      check,
      sleep,
      open,
      setValue,
      key,
      fixtures,
      samples,
      waitFor,
      errors,
      startFrom,
    }) => {
      const setFiles = async (sel, files) => {
        const doc = await send("DOM.getDocument", { depth: -1, pierce: true });
        const q = await send("DOM.querySelector", {
          nodeId: doc.root.nodeId,
          selector: sel,
        });
        await send("DOM.setFileInputFiles", { nodeId: q.nodeId, files });
      };
      const typeInto = (sel, v) => setValue(sel, v);

      // Two files at once: a Word file to translate and a PDF for reference.
      await open("/new", { wait: 0 });
      await sleep(500);
      await click("button", "Continue to files and languages");
      await sleep(300);
      await setFiles("input[type=file][multiple]:not([webkitdirectory])", [
        `${samples}/Mietvertrag_Hofmann_2026.docx`,
        `${fixtures}/Styleguide.pdf`,
      ]);
      check(
        await waitFor(
          "document.querySelectorAll('.file-table tbody tr').length === 2",
        ),
        "both files listed",
      );
      check(
        await evaluate(
          "(() => { const rows = [...document.querySelectorAll('.file-table tbody tr')]; const pdf = rows.find(r => r.textContent.includes('Styleguide.pdf')); const sel = pdf.querySelector('select'); return sel.value === 'reference' && sel.disabled && pdf.textContent.includes('added as a reference file'); })()",
        ),
        "an unreadable PDF becomes a reference file",
      );
      check(
        (await text()).includes("Detected German"),
        "source language detected from the file to translate",
      );
      await setValue("#add-lang", "fr-FR");
      await click(".add-lang button", "Add");
      await sleep(200);
      await typeInto("#pname", "Mietvertrag DE-EN-FR");
      await typeInto("#client", "Kanzlei Weber");
      await typeInto("#due", "2026-12-18T16:00");
      await sleep(300);
      check(
        !(await evaluate(
          "[...document.querySelectorAll('button')].find(b => b.textContent.includes('Continue to memories')).disabled",
        )),
        "files and languages complete",
      );
      await click("button", "Continue to memories");
      await sleep(400);
      let t = await text();
      check(
        t.includes("German (Germany) to English (United Kingdom)") &&
          t.includes("German (Germany) to French (France)"),
        "a memory section per target language",
      );
      await setValue(
        "input[aria-label='Penalty for Kanzlei Weber (DE-EN), in points']",
        "3",
      );
      // A new, empty memory for French.
      await evaluate(
        "[...document.querySelectorAll('.memory-set')].find(f => f.textContent.includes('French')).querySelector('.memory-add .btn-quiet').click()",
      );
      await sleep(200);
      await setValue("#add-mem-fr-FR-name", "Weber DE-FR");
      await click("button", "Create memory");
      await sleep(300);
      t = await text();
      check(
        (
          await evaluate(
            "[...document.querySelectorAll('.memory-set')].find(f => f.textContent.includes('French')).querySelector('.memory-table')?.textContent ?? ''",
          )
        ).includes("Weber DE-FR"),
        "new memory created and chosen for French",
      );
      await click("button", "Continue to terminology");
      await sleep(400);
      t = await text();
      check(
        t.includes("Kanzlei Weber has no French (France)") &&
          t.includes(
            "Otherwise Kanzlei Weber is used for the languages it has",
          ),
        "a master's missing language is a choice, not a side effect",
      );
      await click("button", "Continue to review");
      check(
        await waitFor("!!document.querySelector('.analysis')", 15000),
        "review analyses the files",
      );
      t = await text();
      check(
        t.includes("1 file to translate, 1 reference file") &&
          t.includes("Kanzlei Weber (DE-EN) (−3)") &&
          t.includes("Weber DE-FR"),
        "review summarises files and memories with the penalty",
      );
      const an = await evaluate(
        "document.querySelector('.analysis').innerText",
      );
      check(
        /Context match\s+[^\n]*\s+0\s+0/.test(an) &&
          /Same text and tags as a memory unit\s+0\s+0/.test(an),
        "with a 3-point penalty no match counts as context or exact: " +
          an.replace(/\s+/g, " ").slice(0, 200),
      );
      check(
        (await evaluate(
          "[...document.querySelectorAll('[role=tab]')].map(b => b.textContent).join(',')",
        )) === "English (United Kingdom),French (France)",
        "an analysis per target language",
      );
      await click(".check", "Fill empty segments with memory matches");
      await sleep(200);
      await setValue("#pt-min", "75");
      await sleep(500);
      t = await text();
      check(
        /English \(United Kingdom\): \d+ segments? filled/.test(t) &&
          t.includes("French (France): 0 segments filled"),
        "pre-translation shows what it will fill: " +
          (t.match(/English \(United Kingdom\): [^.]*\./)?.[0] ?? ""),
      );
      await click(".check", "Save these settings as a template");
      await sleep(100);
      await setValue("#tpl-name", "Weber DE-EN-FR");
      await sleep(300);
      await click("button", "Create project");
      check(
        await waitFor("!!document.querySelector('.editor .seg')", 15000),
        "project opens in the editor",
      );
      t = await body();
      check(
        t.includes("segments pre-translated") &&
          t.includes("template “Weber DE-EN-FR” saved"),
        "creation reports pre-translation and the template",
      );
      check(
        (await evaluate(
          "[...document.querySelectorAll('#ed-lang option')].map(o => o.value).join(',')",
        )) === "en-GB,fr-FR" &&
          !(await evaluate("!!document.querySelector('#ed-file')")),
        "editor switches between the target languages; the reference file is not offered",
      );
      check(
        (await evaluate(
          "document.querySelectorAll('.seg .seg-status .score').length",
        )) > 0,
        "pre-translated segments carry their match score",
      );
      await setValue("#ed-lang", "fr-FR");
      check(
        await waitFor(
          "document.querySelector('#ed-lang')?.value === 'fr-FR' && document.querySelectorAll('.seg-confirmed, .seg-draft').length === 0",
        ),
        "French has its own segments, nothing translated",
      );

      // The project's settings: every choice is here again.
      await click("a", "Project settings");
      await sleep(800);
      t = await text();
      check(
        t.includes("Styleguide.pdf") &&
          t.includes("Reference file") &&
          (await evaluate("document.getElementById('ps-name').value")) ===
            "Mietvertrag DE-EN-FR" &&
          (await evaluate("document.getElementById('ps-due').value")) ===
            "2026-12-18T16:00",
        "settings show the files, name and due date",
      );
      check(
        (await evaluate(
          "document.querySelector(\"input[aria-label='Penalty for Kanzlei Weber (DE-EN), in points']\").value",
        )) === "3",
        "settings show the memory penalty",
      );
      // Remove French: a warning, then discard.
      await evaluate(
        "[...document.querySelectorAll('.target-list label')].find(l => l.textContent.includes('French')).querySelector('input').click()",
      );
      await sleep(300);
      check(
        !(await text()).includes("drops") &&
          !(await evaluate(
            "[...document.querySelectorAll('button')].find(b => b.textContent === 'Save changes').disabled",
          )),
        "removing an untranslated language needs no warning and can be saved",
      );
      await click("button", "Discard changes");
      await sleep(200);
      // Add Italian: every file gets it.
      await setValue("#add-lang", "it-IT");
      await click(".add-lang button", "Add");
      await sleep(300);
      await click("button", "Save changes");
      check(
        await waitFor(
          "document.body.innerText.includes('Italian (Italy) added to every file')",
        ),
        "a language added in the settings",
      );
      await click("a", "Open in the editor");
      await sleep(800);
      check(
        (await evaluate(
          "[...document.querySelectorAll('#ed-lang option')].map(o => o.value).join(',')",
        )) === "en-GB,fr-FR,it-IT",
        "the editor offers the new language",
      );

      // The template starts the next project with the same settings.
      await open("/settings", { keep: true, wait: 600 });
      check(
        (await text()).includes("Weber DE-EN-FR"),
        "template listed in Settings",
      );
      await startFrom(`${fixtures}/documents/Kaufvertrag_Mueller_Entwurf.txt`);
      await click(".choice", "Template: Weber DE-EN-FR");
      await sleep(200);
      await click("button", "Continue to files and languages");
      await sleep(400);
      check(
        (await evaluate(
          "[...document.querySelectorAll('.target-list input:checked')].map(i => i.closest('label').textContent).join(',')",
        )) === "English (United Kingdom),French (France)",
        "template's target languages chosen",
      );
      await click("button", "Continue to memories");
      await sleep(400);
      check(
        (await evaluate(
          "document.querySelector(\"input[aria-label='Penalty for Kanzlei Weber (DE-EN), in points']\")?.value",
        )) === "3",
        "template's memories with their penalty",
      );

      // Memories page: the new memory is in use and can't be deleted.
      await open("/memories", { keep: true, wait: 800 });
      const row = await evaluate(
        "[...document.querySelectorAll('tr')].find(r => r.querySelector('th')?.textContent.includes('Weber DE-FR'))?.innerText ?? ''",
      );
      check(
        row.includes("Mietvertrag DE-EN-FR") &&
          row.includes("template Weber DE-EN-FR"),
        "memory shows the project and template that use it: " +
          row.replace(/\s+/g, " "),
      );
      check(
        await evaluate(
          "[...document.querySelectorAll('tr')].find(r => r.querySelector('th')?.textContent.includes('Weber DE-FR')).querySelector('button:last-child').disabled",
        ),
        "a memory in use can't be deleted",
      );
      check(
        (await text()).includes("Mietvertrag DE-EN-FR (en-GB)") &&
          (await text()).includes("Mietvertrag DE-EN-FR (it-IT)"),
        "project memories per language, the added one included",
      );

      // Delete the project: its project memories go, the main ones stay.
      await open("/projects", { keep: true, wait: 600 });
      await evaluate(
        "[...document.querySelectorAll('tr')].find(r => r.textContent.includes('Mietvertrag DE-EN-FR')).querySelector('a[aria-label^=Settings]').click()",
      );
      await sleep(800);
      await click("button", "Delete project");
      await sleep(150);
      await click(".confirm button", "Delete");
      await sleep(500);
      check(
        !(await text()).includes("Mietvertrag DE-EN-FR"),
        "deleted project gone from the list",
      );
      await open("/memories", { keep: true, wait: 800 });
      t = await text();
      check(
        !t.includes("Mietvertrag DE-EN-FR (en-GB)") &&
          t.includes("Weber DE-FR"),
        "project memories deleted with the project, main memories kept",
      );

      await evaluate(
        "[...document.querySelectorAll('tr')].find(r => r.querySelector('th')?.textContent.includes('Weber DE-FR')).querySelector('button').click()",
      );
      check(await waitFor("document.querySelector('#localmem-title')?.textContent === 'Weber DE-FR'"), "a memory on this computer opens");

      // The units of a memory on this computer: a unit is added, found, its translation
      // changed, and the unit deleted.
      {
        const panel = "section[aria-labelledby=localmem-title]";
        await click(`${panel} button`, "Add a unit");
        check(await waitFor("!!document.querySelector('dialog[open] textarea')"), "local memory: the dialog for a new unit opens");
        check(await evaluate("[...document.querySelectorAll('dialog[open] button')].find(b => b.textContent.includes('Add the unit')).disabled"), "local memory: a unit needs its sentence and translation");
        await evaluate(
          `(() => { const set = (el, v) => { Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true })); }; const ts = document.querySelectorAll('dialog[open] textarea'); set(ts[0], "Die Hausordnung gilt."); set(ts[1], "Le r\u00e8glement int\u00e9rieur s'applique."); return true; })()`,
        );
        await sleep(100);
        await click("dialog[open] button", "Add the unit");
        check(await waitFor("document.querySelector('.toast')?.textContent.includes('Added the unit to')", 10000), "local memory: the unit is added");
        await setValue(`${panel} input[type=search]`, "Hausordnung");
        if (await evaluate(`document.querySelector(${JSON.stringify(panel)} + ' input[type=checkbox]').checked`))
          await evaluate(`document.querySelector(${JSON.stringify(panel)} + ' input[type=checkbox]').click()`);
        await sleep(100);
        await click(`${panel} button`, "Search");
        check(await waitFor(`document.querySelector(${JSON.stringify(panel)} + ' table[aria-label="Units found"]')?.innerText.includes("Le r\u00e8glement int\u00e9rieur s'applique.")`, 10000), "local memory: the new unit is found");
        await click(`${panel} table button`, "Change");
        check(await waitFor("!!document.querySelector('dialog[open] .unit-target .target-editor')"), "local memory: the unit opens to be changed");
        check((await evaluate("document.querySelector('dialog[open] .unit-source').innerText")).includes("Die Hausordnung gilt."), "local memory: its sentence is shown to read");
        await evaluate("(() => { const el = document.querySelector('dialog[open] .unit-target .target-editor'); el.focus(); const r = document.createRange(); r.selectNodeContents(el); r.collapse(false); const s = getSelection(); s.removeAllRanges(); s.addRange(r); return document.execCommand('insertText', false, ' Geändert.'); })()");
        await sleep(100);
        await click("dialog[open] button", "Save");
        check(await waitFor("document.querySelector('.toast')?.textContent.includes('Saved the unit')", 10000), "local memory: the changed translation is saved");
        check(await waitFor(`document.querySelector(${JSON.stringify(panel)} + ' table[aria-label="Units found"]')?.innerText.includes('Geändert.')`, 10000), "local memory: the search shows the changed translation");
        await evaluate(`document.querySelector(${JSON.stringify(panel)} + ' table .cell-action .row > button:last-child').click()`);
        await sleep(150);
        check((await text()).includes("Delete this unit?"), "local memory: deleting a unit asks first");
        await click(`${panel} table .confirm button`, "Delete");
        check(await waitFor("document.querySelector('.toast')?.textContent.includes('Deleted the unit')", 10000), "local memory: the unit is deleted");
        check(await waitFor(`document.querySelector(${JSON.stringify(panel)}).innerText.includes('Nothing found.')`, 10000), "local memory: and no longer found");
      }
      check(
        errors.length === 0,
        "no console errors in setup: " + errors.join(" | "),
      );

      // Work saved by the first version (state version 1) opens moved into
      // the current shape, with its project memory under its new name.
      await open("/projects", { wait: 600 });
      await evaluate(`(async () => {
        const db = await new Promise((ok, no) => { const r = indexedDB.open('interlinea-prototype', 1); r.onsuccess = () => ok(r.result); r.onerror = () => no(r.error); });
        const put = (k, v) => new Promise((ok, no) => { const tx = db.transaction('kv', 'readwrite'); tx.objectStore('kv').put(v, k); tx.oncomplete = ok; tx.onerror = () => no(tx.error); });
        const seg = (id, src, tgt, status) => ({ id, source: [src], target: tgt ? [tgt] : [], status });
        await put('state', { version: 1, state: {
          termbases: [], entries: [], baseSnapshots: {}, branchedAt: {},
          gitWords: false, autoInsert: true, autoPropagate: true, contextRule: 'both',
          projects: [{
            id: 'p-old', name: 'Alter Vertrag', file: 'alt.txt', words: 6,
            sourceLang: 'de-DE', targetLangs: ['en-GB', 'fr-FR'], client: 'Kanzlei Weber',
            setup: { mode: 'own', masters: [], location: 'local' },
            createdAt: '2026-10-01T09:00:00.000Z', progress: 0.5,
            segments: [seg('s1', 'Erster Satz.', 'First sentence.', 'confirmed'), seg('s2', 'Zweiter Satz.', '', 'new')],
          }],
        } });
        const tmx = '<?xml version="1.0" encoding="UTF-8"?><tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="de-DE" datatype="plaintext"/><body><tu><tuv xml:lang="de-DE"><seg>Alt.</seg></tuv><tuv xml:lang="en-GB"><seg>Old.</seg></tuv></tu></body></tmx>';
        await put('memory:project:p-old', new TextEncoder().encode(tmx));
        // This page saves its own state when it unloads; it must not
        // overwrite the old save before the next page load reads it.
        IDBObjectStore.prototype.put = function () { return this.count(); };
        return true;
      })()`);
      await open("/projects", { keep: true, wait: 1200 });
      t = await text();
      check(t.includes("Alter Vertrag"), "first-version project opens");
      await open("/editor/p-old/f-p-old/fr-FR", { keep: true, wait: 1500 });
      check(
        (await evaluate("document.querySelectorAll('.seg').length")) === 2 &&
          (await evaluate(
            "document.querySelectorAll('.seg-confirmed').length",
          )) === 0,
        "its second language got fresh segments",
      );
      await open("/editor/p-old/f-p-old/en-GB", { keep: true, wait: 1500 });
      check(
        (await evaluate(
          "document.querySelectorAll('.seg-confirmed').length",
        )) === 1,
        "its first language kept the translation",
      );
      await click("button", "Memories");
      await sleep(400);
      const mem = await evaluate(
        "document.querySelector('dialog[open]')?.innerText ?? ''",
      );
      check(
        /Alter Vertrag \(en-GB\) \(project memory\)\s+lookup, concordance, updated on confirm\s+2/.test(
          mem,
        ),
        "its project memory kept the imported unit and gained the confirmed one: " +
          mem.replace(/\s+/g, " ").slice(200, 520),
      );
      await evaluate("document.querySelector('dialog[open]').close()");
      await open("/project/p-old", { keep: true, wait: 800 });
      check(
        (
          await evaluate(
            "document.querySelector('input[name=ctx]:checked')?.closest('label').textContent ?? ''",
          )
        ).includes("before and after"),
        "its context rule came from the saved setting",
      );
      check(
        errors.length === 0,
        "no console errors after migration: " + errors.join(" | "),
      );

      // Spelling with a dictionary added in Settings, a rule of the
      // project's own, and the length limit an XLIFF file sets.
      await open("/settings", { wait: 600 });
      await setFiles("input[accept*='.oxt']", [
        `${fixtures}/en_GB.aff`,
        `${fixtures}/en_GB.dic`,
      ]);
      check(
        await waitFor(
          "document.body.innerText.includes('Found in the files') && document.getElementById('dl-0')?.value === 'en-GB'",
        ),
        "dictionary found, its language read from the file name",
      );
      await click("button", "Add 1 dictionary");
      check(
        await waitFor("document.body.innerText.includes('12 words')"),
        "dictionary loaded by the core and listed",
      );
      await open("/new", { keep: true, wait: 600 });
      await click("button", "Continue to files and languages");
      await sleep(300);
      await setFiles("input[type=file][multiple]:not([webkitdirectory])", [
        `${fixtures}/Hinweise.xlf`,
      ]);
      await waitFor(
        "document.querySelectorAll('.file-table tbody tr').length === 1",
      );
      await click("button", "Continue to memories");
      await sleep(300);
      await click("button", "Continue to terminology");
      await sleep(300);
      await click("button", "Continue to review");
      await waitFor("!!document.querySelector('.analysis')");
      await click("summary", "Checks and context matches");
      check(
        await waitFor("document.getElementById('dict-en-GB')?.value === 'en_GB'"),
        "the dictionary for English is chosen by itself",
      );
      await click("button", "Add a rule");
      await sleep(150);
      await setValue("#rule-0-name", "No contractions");
      await setValue("#rule-0-target", "(");
      await sleep(400);
      check(
        (await text()).includes(
          'Rule "No contractions": error parsing regexp',
        ) &&
          (await evaluate(
            "[...document.querySelectorAll('button')].find(b => b.textContent === 'Create project').disabled",
          )),
        "a pattern the core can't compile is refused",
      );
      await setValue("#rule-0-target", "n't");
      await setValue("#rule-0-msg", "Write it out in full.");
      await sleep(400);
      await click("button", "Create project");
      check(
        await waitFor("!!document.querySelector('.editor .seg')", 15000),
        "XLIFF project opens",
      );
      await evaluate(
        "[...document.querySelectorAll('.seg')][0].querySelector('.seg-target-view, .target-editor').click()",
      );
      await sleep(400);
      check(
        (await evaluate(
          "document.querySelector('.length-counter')?.textContent",
        )) === "24 of 30 characters",
        "the file's length limit is counted",
      );
      await key("a", "KeyA", 65, 2);
      await send("Input.insertText", {
        text: "The tenant doesn't payz the rent monthly.",
      });
      check(
        await waitFor("[...document.querySelectorAll('.issue')].length >= 3"),
        "checks run",
      );
      const qaText = await evaluate(
        "[...document.querySelectorAll('.issue')].map(i => i.textContent).join(' | ')",
      );
      check(
        qaText.includes(
          "“payz” is not in the dictionary. Perhaps pay, pays.",
        ) &&
          qaText.includes("Write it out in full.") &&
          qaText.includes("at most 30 are allowed") &&
          (await evaluate(
            "document.querySelector('.length-counter').classList.contains('is-over')",
          )),
        "spelling, the project's rule and the length limit flagged: " + qaText,
      );
      check(
        !qaText.includes("“tenant”") && !qaText.includes("“monthly”"),
        "words in the dictionary pass",
      );
      await evaluate(
        "document.querySelector(\"button[aria-label^='Add payz to the project']\").click()",
      );
      check(
        await waitFor(
          "![...document.querySelectorAll('.issue')].some(i => i.textContent.includes('payz'))",
        ),
        "a word added to the project's words passes",
      );
      await click("a", "Project settings");
      await sleep(600);
      check(
        (await evaluate(
          "[...document.querySelectorAll('.word-list .tag')].map(t => t.textContent).join()",
        )) === "payz",
        "the project's words are in its settings",
      );
      check(
        errors.length === 0,
        "no console errors with spelling: " + errors.join(" | "),
      );
    },
  );
