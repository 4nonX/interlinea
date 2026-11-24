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
      downloads,
      dlg,
      setFile,
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

      // A document and its translation, done before, are aligned into the
      // memory: the sentences are paired with lines between the two texts,
      // lines are taken away and drawn, and what is paired is added.
      {
        const panel = "section[aria-labelledby=localmem-title]";
        const status = () => evaluate("document.querySelector('dialog[open] [role=status]')?.innerText ?? ''");
        const lines = () => evaluate("[...document.querySelectorAll('dialog[open] svg [data-line]')].map(g => g.dataset.line).sort().join(' ')");
        const seg = (side, i) => `dialog[open] button[data-side=${side}][data-i="${i}"]`;
        const choose = async (side, i) => {
          await evaluate(`document.querySelector(${JSON.stringify(seg(side, i))}).click()`);
          await sleep(80);
        };
        await click(`${panel} button`, "Align documents");
        check(await waitFor("document.querySelectorAll('dialog[open] input[type=file]').length === 2"), "alignment: the dialog asks for a document and its translation");
        check(await evaluate("[...document.querySelectorAll('dialog[open] button')].find(b => b.textContent.includes('Pair the sentences')).disabled"), "alignment: both files are needed");
        const d = (await dlg());
        check(d.includes("Document in German") && d.includes("Its translation in French"), "alignment: the files are asked for by the memory's languages");
        await setFile("dialog[open] label:nth-of-type(1) input[type=file]", `${fixtures}/align/Mietvertrag_de.txt`);
        await setFile("dialog[open] label:nth-of-type(2) input[type=file]", `${fixtures}/align/Mietvertrag_fr.txt`);
        await click("dialog[open] button", "Pair the sentences");
        check(await waitFor("document.querySelectorAll('dialog[open] svg [data-line]').length === 5", 20000), "alignment: the two texts are shown with lines between them");
        check((await status()).startsWith("4 units from 5 and 4 segments. All of them fit."), "alignment: four units of five and four segments: " + (await status()));
        check((await lines()) === "0|0 1|1 2|2 3|2 4|3", "alignment: two sentences translated as one both lead to it: " + (await lines()));
        check(
          (await evaluate(`document.querySelector(${JSON.stringify(seg("source", 1))}).innerText`)).includes("Die Kaution beträgt 2.400,00 Euro.") &&
            (await evaluate(`document.querySelector(${JSON.stringify(seg("target", 1))}).innerText`)).includes("2 400,00 euros"),
          "alignment: the numbers pair in either notation",
        );
        check(
          await evaluate(`(() => {
            const l = document.querySelector('dialog[open] svg [data-line="0|0"] .align-line');
            const n = l.getTotalLength();
            const p = l.getPointAtLength(0), q = l.getPointAtLength(n), nearP = l.getPointAtLength(4), nearQ = l.getPointAtLength(n - 4);
            const a = document.querySelector(${JSON.stringify(seg("source", 0))}).getBoundingClientRect();
            const b = document.querySelector(${JSON.stringify(seg("target", 0))}).getBoundingClientRect();
            const s = document.querySelector('dialog[open] svg').getBoundingClientRect();
            return l.tagName === 'path' && Math.abs(p.x + s.left - a.right) < 2 && Math.abs(q.x + s.left - b.left) < 2 && Math.abs(p.y + s.top - (a.top + a.height / 2)) < 2 && Math.abs(q.y + s.top - (b.top + b.height / 2)) < 2;
          })()`),
          "alignment: a line runs from the sentence to its translation",
        );
        check(
          await evaluate(`(() => {
            const l = document.querySelector('dialog[open] svg [data-line="3|2"] .align-line');
            const n = l.getTotalLength();
            const p = l.getPointAtLength(0), q = l.getPointAtLength(n), nearP = l.getPointAtLength(5), nearQ = l.getPointAtLength(n - 5);
            return Math.abs(p.y - q.y) > 15 && Math.abs(nearP.y - p.y) < 1 && Math.abs(nearQ.y - q.y) < 1;
          })()`),
          "alignment: between segments at different heights it is a curve that leaves and arrives level",
        );
        check(
          await evaluate(`(() => {
            const ok = (side, i) => {
              const b = document.querySelector('dialog[open] button[data-side=' + side + '][data-i="' + i + '"]');
              const r = b.getBoundingClientRect(), p = b.querySelector('.align-port').getBoundingClientRect();
              const beside = side === 'source' ? p.left >= r.right && p.left - r.right < 4 : p.right <= r.left && r.left - p.right < 4;
              return beside && Math.abs(p.top + p.height / 2 - (r.top + r.height / 2)) < 2 && p.width >= 8;
            };
            return document.querySelectorAll('dialog[open] .align-port').length === 9 && ok('source', 0) && ok('target', 3);
          })()`),
          "alignment: every segment has a point in the gap beside it, where its lines begin",
        );
        check((await dlg()).includes("Add 4 units to Weber DE-FR"), "alignment: all four would be added");
        // What was paired for the translator is dotted. A pair confirmed is
        // solid and stays when the rest is paired again around it.
        const mine = () => evaluate("[...document.querySelectorAll('dialog[open] svg .align-line.is-mine')].map(l => l.parentElement.dataset.line).sort().join(' ')");
        const enabled = (label) => evaluate(`![...document.querySelectorAll('dialog[open] button')].find(b => b.textContent.trim() === ${JSON.stringify(label)}).disabled`);
        check((await mine()) === "" && !(await enabled("Confirm")) && !(await enabled("Pair the rest again")), "alignment: nothing is checked at first");
        check(await evaluate("getComputedStyle(document.querySelector('dialog[open] svg [data-line=\"1|1\"] .align-line')).strokeDasharray !== 'none'"), "alignment: a line paired for the translator is dotted");
        await choose("source", 1);
        check(await enabled("Confirm"), "alignment: a pair chosen can be confirmed");
        await click("dialog[open] button", "Confirm");
        check(await waitFor("document.querySelectorAll('dialog[open] svg .align-line.is-mine').length === 1"), "alignment: confirmed, its line is the translator's own");
        check((await mine()) === "1|1" && (await evaluate("getComputedStyle(document.querySelector('dialog[open] svg [data-line=\"1|1\"] .align-line')).strokeDasharray === 'none'")), "alignment: and drawn solid");
        check((await status()).includes("1 unit is checked.") && (await status()).includes("All of them fit."), "alignment: the status counts what is checked: " + (await status()));
        await evaluate("document.querySelector('dialog[open] svg [data-line=\"0|0\"] .align-hit').dispatchEvent(new MouseEvent('click', { bubbles: true }))");
        await evaluate("document.querySelector('dialog[open] svg [data-line=\"4|3\"] .align-hit').dispatchEvent(new MouseEvent('click', { bubbles: true }))");
        check(await waitFor("document.querySelectorAll('dialog[open] svg [data-line]').length === 3") && (await enabled("Pair the rest again")), "alignment: with a pair checked the rest can be paired again");
        await click("dialog[open] button", "Pair the rest again");
        check(await waitFor("document.querySelectorAll('dialog[open] svg [data-line]').length === 5", 20000), "alignment: the rest is paired again");
        check((await lines()) === "0|0 1|1 2|2 3|2 4|3" && (await mine()) === "1|1", "alignment: around the checked pair, which stays the translator's own: " + (await mine()));
        // A sentence chosen and then its translation takes their line away.
        await choose("source", 3);
        check(await evaluate(`document.querySelector(${JSON.stringify(seg("source", 3))}).getAttribute('aria-pressed') === 'true'`), "alignment: a sentence is chosen");
        await choose("target", 2);
        check((await lines()) === "0|0 1|1 2|2 4|3", "alignment: choosing both ends takes the line away");
        check((await status()).includes("4 units") && (await status()).includes("1 segment has no pair and is left out."), "alignment: the sentence without a line is left out: " + (await status()));
        check(await evaluate(`document.querySelector(${JSON.stringify(seg("source", 3))}).classList.contains('is-alone')`), "alignment: and is marked");
        check(
          await evaluate(`(() => { const css = (i) => getComputedStyle(document.querySelector('dialog[open] button[data-side=source][data-i="' + i + '"] .align-port')); return css(3).backgroundColor !== css(2).backgroundColor && css(3).borderColor !== css(2).borderColor; })()`),
          "alignment: its point is a ring while nothing is joined there",
        );
        // The same draws it again.
        await choose("target", 2);
        await choose("source", 3);
        check((await lines()) === "0|0 1|1 2|2 3|2 4|3" && (await status()).includes("All of them fit."), "alignment: choosing both ends draws the line");
        check((await mine()) === "1|1 2|2 3|2", "alignment: a line drawn by hand is the translator's own, with the unit it joins: " + (await mine()));
        // A line is taken away by choosing it, and drawn by pulling from one text to the other.
        await evaluate("document.querySelector('dialog[open] svg [data-line=\"0|0\"] .align-hit').dispatchEvent(new MouseEvent('click', { bubbles: true }))");
        check(await waitFor("document.querySelectorAll('dialog[open] svg [data-line]').length === 4"), "alignment: a line chosen is taken away");
        check((await status()).includes("3 units") && (await status()).includes("2 segments have no pair and are left out.") && (await dlg()).includes("Add 3 units"), "alignment: its two ends are left out: " + (await status()));
        await evaluate(`(() => { const a = document.querySelector(${JSON.stringify(seg("source", 0))}); const b = document.querySelector(${JSON.stringify(seg("target", 0))}).getBoundingClientRect(); a.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true })); document.querySelector('dialog[open] .align-board').dispatchEvent(new PointerEvent('pointerup', { bubbles: true, clientX: b.left + b.width / 2, clientY: b.top + b.height / 2 })); return true; })()`);
        check(await waitFor("document.querySelectorAll('dialog[open] svg [data-line]').length === 5"), "alignment: pulling from a sentence to a translation draws a line");
        // A sentence joined to a second translation makes one unit of all three.
        await choose("source", 0);
        await choose("target", 1);
        check((await status()).startsWith("3 units") && (await dlg()).includes("Add 3 units"), "alignment: segments joined through lines are one unit: " + (await status()));
        await choose("source", 0);
        await choose("target", 1);
        // The first sentence is left out.
        await choose("source", 0);
        await choose("target", 0);
        check(await waitFor("document.querySelector('dialog[open]').innerText.includes('Add 3 units to Weber DE-FR')"), "alignment: a pair without its line is left out");
        await click("dialog[open] button", "Add 3 units");
        check(await waitFor("document.querySelector('.toast')?.textContent.includes('Added 3 units to Weber DE-FR')", 10000), "alignment: the units are added");
        check(!(await evaluate("!!document.querySelector('dialog[open]')")), "alignment: the dialog closes");
        await setValue(`${panel} input[type=search]`, "Kaution");
        await sleep(100);
        await click(`${panel} button`, "Search");
        check(await waitFor(`document.querySelector(${JSON.stringify(panel)} + ' table[aria-label="Units found"]')?.innerText.includes("Le dépôt de garantie s'élève à 2 400,00 euros.")`, 10000), "alignment: an aligned unit is found in the memory");
        await setValue(`${panel} input[type=search]`, "Vermögen");
        await sleep(100);
        await click(`${panel} button`, "Search");
        check(await waitFor(`document.querySelector(${JSON.stringify(panel)} + ' table[aria-label="Units found"]')?.innerText.includes("Sie wird bei Beginn des Mietverhältnisses fällig. Der Vermieter legt sie getrennt von seinem Vermögen an.")`, 10000), "alignment: two sentences with one translation are one unit");
        await setValue(`${panel} input[type=search]`, "Werktag");
        await sleep(100);
        await click(`${panel} button`, "Search");
        check(await waitFor(`document.querySelector(${JSON.stringify(panel)}).innerText.includes('Nothing found.')`, 10000), "alignment: the sentence left out is not");
        // A file that is no text says so.
        await click(`${panel} button`, "Align documents");
        await waitFor("document.querySelectorAll('dialog[open] input[type=file]').length === 2");
        await setFile("dialog[open] label:nth-of-type(1) input[type=file]", `${fixtures}/Styleguide.pdf`);
        await setFile("dialog[open] label:nth-of-type(2) input[type=file]", `${fixtures}/align/Mietvertrag_fr.txt`);
        await click("dialog[open] button", "Pair the sentences");
        check(await waitFor("(document.querySelector('dialog[open] [role=alert]')?.innerText ?? '').includes('Styleguide.pdf could not be read')", 10000), "alignment: a file that can't be read says so");
        await click("dialog[open] button", "Cancel");
        await sleep(150);
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

      // The settings page analyses the project's files and saves the count.
      await click("button", "Analyse the files");
      check(
        await waitFor("!!document.querySelector('table[aria-label^=\"Match analysis\"], table caption')", 10000),
        "the settings page analyses the files",
      );
      const analysed = await evaluate("document.querySelector('#project-analysis').closest('section').innerText");
      check(
        analysed.includes("Analyse again") && analysed.includes("counted in this order") && /Total\s+\d+/.test(analysed),
        "the analysis says what it was counted against, with a total: " + analysed.replace(/\s+/g, " ").slice(0, 160),
      );
      await click("button", "Save as CSV");
      await sleep(800);
      const csvName = (await import("node:fs")).readdirSync(downloads).find((f) => f.endsWith("analysis.csv"));
      const csv = csvName ? (await import("node:fs")).readFileSync(`${downloads}/${csvName}`, "utf8") : "";
      check(
        csv.includes('"Language","File","Match","Segments","Words","Characters"') && /"en-GB","[^"]+","[^"]+",\d+,\d+,\d+/.test(csv),
        "the analysis is saved as CSV: " + (csvName ?? "no file"),
      );
      check(
        errors.length === 0,
        "no console errors with spelling: " + errors.join(" | "),
      );
    },
  );
