// Projects, editor, memories, merge and settings
import { run } from "./harness.mjs";

export default () =>
  run(
    "Projects, editor, memories, merge and settings",
    async ({
      send,
      evaluate,
      click,
      text,
      body,
      check,
      sleep,
      open,
      errors,
      fixtures,
      samples,
    }) => {
      await open("/projects", { wait: 0 });
      await sleep(800);

      // New project from the German sample.
      await click("button", "Kaufvertrag_Mueller_Entwurf.docx");
      await sleep(300);
      let t = await text();
      check(t.includes("A blank project"), "new project starts at Start");
      await click("button", "Continue to files and languages");
      await sleep(400);
      t = await text();
      check(
        t.includes("Detected German (Germany)"),
        "German detected for Kaufvertrag sample",
      );
      await click("button", "Continue to memories");
      await sleep(400);
      check(
        /Kanzlei Weber \(DE-EN\)/.test(await text()) &&
          (await evaluate(
            "[...document.querySelectorAll('.memory-table input[type=checkbox]')].map(i => i.checked).join()",
          )) === "true,true,false",
        "client memory suggested for lookup and concordance, not updated (a project memory collects)",
      );
      await click("button", "Continue to terminology");
      await sleep(300);
      check(
        await evaluate(
          `[...document.querySelectorAll('.choice')].find(c => c.textContent.includes('Kanzlei Weber')).querySelector('input').checked`,
        ),
        "Kanzlei Weber preselected for client",
      );
      t = await text();
      check(
        t.includes("Suggested for Kanzlei Weber"),
        "suggestion label shown",
      );
      check(
        await evaluate(
          `document.querySelector('input[name=mode][value=layered]').checked`,
        ),
        "layered mode default",
      );
      await click(".choice", "Mietrecht");
      await sleep(200);
      t = await text();
      check(
        t.includes("Mietrecht has no") === false,
        "Mietrecht has de/en: no missing-language note",
      );
      await click("button", "Continue to review");
      await sleep(200);
      t = await text();
      check(
        t.includes("based on Kanzlei Weber, Mietrecht"),
        "review summarises masters: " +
          t.split("Terminology")[2]?.slice(0, 120),
      );
      await click("button", "Create project");
      await sleep(500);
      t = await text();
      check(
        t.includes("Kaufvertrag Mueller Entwurf") &&
          (await evaluate("document.querySelectorAll('.seg').length")) > 3,
        "new project opens in the editor with segments: " +
          (await evaluate("document.querySelectorAll('.seg').length")),
      );
      check(
        await evaluate(
          "document.activeElement?.classList.contains('target-editor')",
        ),
        "caret is in the first target",
      );
      await click("a", "Projects");
      await sleep(400);
      t = await text();
      check(
        t.includes("Kaufvertrag Mueller Entwurf"),
        "new project listed on Projects page",
      );

      // Korean sample: script detection and missing-language note.
      await click("button", "임대차계약서_서울.txt");
      await sleep(300);
      await click("button", "Continue to files and languages");
      await sleep(300);
      t = await text();
      check(t.includes("Detected Korean"), "Korean detected");
      // Short English sample: English detected
      await open("/new/sample/1", { wait: 0 });
      await sleep(600);
      await click("button", "Continue to files and languages");
      await sleep(300);
      t = await text();
      check(
        t.includes("Detected English"),
        "English detected: " +
          (t.match(/Detected[^\n]*/)?.[0] ?? t.match(/Probably[^\n]*/)?.[0]),
      );
      await open("/new/sample/2", { wait: 0 });
      await sleep(600);
      await click("button", "Continue to files and languages");
      await sleep(300);
      t = await text();
      check(
        t.includes("Detected French"),
        "French detected: " +
          (t.match(/Detected[^\n]*/)?.[0] ?? t.match(/Probably[^\n]*/)?.[0]),
      );

      // Translation editor.
      const key = async (k, code, vk, modifiers = 0) => {
        await send("Input.dispatchKeyEvent", {
          type: "keyDown",
          key: k,
          code,
          windowsVirtualKeyCode: vk,
          modifiers,
        });
        await send("Input.dispatchKeyEvent", {
          type: "keyUp",
          key: k,
          code,
          windowsVirtualKeyCode: vk,
          modifiers,
        });
        await sleep(250);
      };
      const activeNum = () =>
        evaluate(
          "document.querySelector('.seg.is-active .seg-num').textContent",
        );
      const openSeg = (n) =>
        evaluate(
          `[...document.querySelectorAll('.seg')].find(s => s.querySelector('.seg-num').textContent === '${n}').querySelector('.seg-target-view, .target-editor').click()`,
        ).then(() => sleep(250));
      await open("/projects", { wait: 0 });
      for (
        let i = 0;
        i < 40 &&
        !(await evaluate("!!document.querySelector('a.cell-link')").catch(
          () => false,
        ));
        i++
      )
        await sleep(100);
      await click("a.cell-link", "Mietvertrag Hofmann");
      await sleep(500);
      check(
        (await evaluate("document.querySelectorAll('.seg').length")) === 38,
        "sample has the 38 units the Word filter extracts",
      );
      check(
        (await activeNum()) === "8",
        "editor opens on the first unconfirmed segment",
      );
      check(
        (await evaluate(
          "document.querySelectorAll('.seg.is-active .term-hit').length",
        )) >= 1,
        "term recognised in active source",
      );
      check(
        (await evaluate(
          "[...document.querySelectorAll('.seg')][1].querySelectorAll('.seg-source .tag-chip').length",
        )) === 4,
        "bold names in segment 2 come from the file as tags",
      );
      await key("ArrowDown", "ArrowDown", 40, 1);
      await key("ArrowDown", "ArrowDown", 40, 1);
      check((await activeNum()) === "10", "Alt+Down moves to segment 10");
      check(
        (
          await evaluate("document.querySelector('.target-editor').innerText")
        ).includes("980.00"),
        "best match inserted when an empty segment opens",
      );
      check(
        (await evaluate(
          "document.querySelectorAll('.target-editor .tag-chip').length",
        )) === 4,
        "inserted match brings its bold tags",
      );
      const badge = await evaluate(
        "document.querySelector('.seg.is-active .seg-status .score > [aria-hidden]')?.textContent ?? ''",
      );
      check(
        /^\d\d%/.test(badge) &&
          (await evaluate(
            "document.querySelector('.seg.is-active .seg-status .score').className.includes('band-8')",
          )),
        "fuzzy score badge with its band: " + badge,
      );
      const before = await evaluate(
        "document.querySelector('.progress-text').textContent",
      );
      await key("Enter", "Enter", 13, 2);
      const after = await evaluate(
        "document.querySelector('.progress-text').textContent",
      );
      check(
        before !== after,
        "Ctrl+Enter confirms and updates progress: " + before + " -> " + after,
      );
      check((await activeNum()) === "11", "moved to the next open segment");
      check(
        await evaluate(
          "[...document.querySelectorAll('.seg')][9].querySelector('.seg-warn')?.textContent.includes('3')",
        ),
        "segment 10 confirmed with warnings",
      );
      check(
        (
          await evaluate("document.querySelector('[role=status]').textContent")
        ).startsWith("Confirmed with 3 warnings"),
        "confirmation announced with warnings",
      );
      check(
        (await text()).includes("Warnings (1)"),
        "warnings filter counts segments",
      );
      check(
        (
          await evaluate("document.querySelector('.target-editor').innerText")
        ).trim() === "Basic rent" &&
          (await evaluate(
            "document.querySelector('.seg.is-active .seg-status .score > [aria-hidden]').textContent",
          )) === "100%",
        "table cell gets the 100% match",
      );
      // Ctrl+number inserts a result.
      await evaluate(
        "document.querySelector('.target-editor').textContent = ''; document.querySelector('.target-editor').dispatchEvent(new Event('input', { bubbles: true }))",
      );
      await sleep(150);
      await key("1", "Digit1", 49, 2);
      check(
        (
          await evaluate("document.querySelector('.target-editor').innerText")
        ).includes("Basic rent"),
        "Ctrl+1 inserts the first result",
      );
      await send("Input.insertText", { text: " (net)" });
      await sleep(200);
      check(
        await evaluate(
          "document.querySelector('.seg.is-active').classList.contains('seg-draft') && document.querySelector('.seg.is-active .score').classList.contains('is-edited')",
        ),
        "editing keeps a draft and marks the score as edited",
      );
      // Context match: segment 18 follows the same sentence as in the memory.
      await openSeg(18);
      check(
        (await evaluate(
          "document.querySelector('.result .score > [aria-hidden]')?.textContent",
        )) === "CM" &&
          (await evaluate(
            "document.querySelector('.seg.is-active .seg-status .score > [aria-hidden]')?.textContent",
          )) === "CM",
        "context match shown as CM",
      );
      await openSeg(29);
      check(
        (await evaluate(
          "document.querySelector('.result .score > [aria-hidden]')?.textContent",
        )) === "100%",
        "same text after a different sentence is 100%, not CM",
      );
      // Ignore a warning on segment 10.
      await openSeg(10);
      const issueText = await evaluate(
        "[...document.querySelectorAll('.issue')].map(i => i.textContent).join(' | ')",
      );
      check(
        (await evaluate("document.querySelectorAll('.issue').length")) === 3 &&
          issueText.includes(
            "1.250,00 and 220,00 from the source are not in the translation",
          ) &&
          issueText.includes("980.00 and 180.00 are not in the source") &&
          issueText.includes("89% match is unchanged"),
        "segment 10 shows the core's checks (numbers missing and extra) and the unchanged match: " +
          issueText,
      );
      await click(".issue button", "");
      await sleep(200);
      check(
        (await evaluate("document.querySelectorAll('.issue').length")) === 2 &&
          (await text()).includes("1 warning ignored"),
        "ignore removes a false alarm",
      );
      check(
        await evaluate(
          "[...document.querySelectorAll('.seg')][9].querySelector('.seg-warn')?.textContent.includes('2')",
        ),
        "segment warning count drops to 2",
      );
      // Repetitions: "Ort, Datum" in both signature cells.
      await openSeg(31);
      check(
        (
          await evaluate("document.querySelector('.target-editor').innerText")
        ).trim() === "Place, date",
        "signature cell gets its 100% match",
      );
      await key("Enter", "Enter", 13, 2);
      check(
        await evaluate(
          "[...document.querySelectorAll('.seg')][31].classList.contains('seg-confirmed') && [...document.querySelectorAll('.seg')][31].innerText.includes('Place, date')",
        ),
        "confirming fills the repetition (segment 32)",
      );
      check(
        (await evaluate("document.body.innerText")).includes(
          "Also filled 1 repetition (segment 32)",
        ),
        "repetition toast",
      );
      check((await activeNum()) === "33", "skips the filled repetition");
      // Memory search.
      await key("k", "KeyK", 75, 2);
      await evaluate(
        `(() => { const el = document.getElementById('lookup-q'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, 'Kaution'); el.dispatchEvent(new Event('input', { bubbles: true })); })()`,
      );
      await sleep(200);
      check(
        (await evaluate(
          "document.querySelectorAll('.lookup-list li').length",
        )) >= 2,
        "Ctrl+K searches the memory: " +
          (await evaluate(
            "document.querySelectorAll('.lookup-list li').length",
          )) +
          " results",
      );
      await key("Escape", "Escape", 27);
      check(
        await evaluate(
          "!document.getElementById('lookup-q') && document.activeElement?.classList.contains('target-editor')",
        ),
        "Esc closes the search and returns to the target",
      );
      // Tags with F9: segment 24 has italics; its fuzzy match has none.
      await openSeg(24);
      check((await text()).includes("F9"), "missing tags reported in checks");
      await key("F9", "F9", 120);
      check(
        (await evaluate(
          "document.querySelectorAll('.target-editor .tag-chip').length",
        )) === 1,
        "F9 inserts the next tag",
      );
      // Add a term with Ctrl+E.
      await key("e", "KeyE", 69, 2);
      await evaluate(
        `(() => { const set = (id, v) => { const el = document.getElementById(id); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true })); }; set('cap-src', 'Vorauszahlung'); set('cap-tgt', 'advance payment'); })()`,
      );
      await sleep(100);
      await click(".capture button[type=submit]");
      await sleep(300);
      check(
        (await evaluate("document.body.innerText")).includes(
          "Added “Vorauszahlung”",
        ),
        "term added toast",
      );
      await openSeg(10);
      check(
        await evaluate(
          "[...document.querySelectorAll('.result')].some(r => r.textContent.includes('advance payment'))",
        ),
        "new term recognised right away in segment 10",
      );
      // Analysis.
      await click("button", "Analysis");
      await sleep(200);
      const an = await evaluate(
        "document.querySelector('dialog[open]')?.innerText ?? ''",
      );
      check(
        /Context match[\s\S]*?\n?\s*1\s/.test(an) &&
          an.includes("Repetitions") &&
          /Total\s+38/.test(an),
        "analysis dialog: " + an.replace(/\s+/g, " ").slice(0, 160),
      );
      await click("dialog[open] button", "Close");
      await sleep(150);
      // Preview.
      await click("button", "Preview");
      await sleep(300);
      const pv = await evaluate(
        "document.querySelector('.pv-page')?.innerText ?? ''",
      );
      check(
        pv.includes("Residential lease agreement") &&
          pv.includes("Mr Jonas Hofmann") &&
          pv.includes("Lease Hofmann") === false &&
          pv.includes("Seite"),
        "preview shows translations, source where untranslated",
      );
      check(
        (await evaluate("document.querySelectorAll('.pv-table').length")) ===
          2 &&
          (await evaluate("!!document.querySelector('.pv-p strong')")) &&
          (await evaluate(
            "document.querySelectorAll('.pv-notes .pv-p').length",
          )) === 2,
        "preview keeps tables, bold and footnotes",
      );
      await evaluate(
        "[...document.querySelectorAll('.pv-seg')].find(s => s.textContent.includes('Kaution in Höhe')).click()",
      );
      await sleep(250);
      check(
        (await activeNum()) === "20",
        "clicking text in the preview opens its segment",
      );
      // Save translated document: capture the download and read it back with the core.
      await evaluate(
        "window.__saved = null; const orig = URL.createObjectURL; URL.createObjectURL = (b) => { window.__saved = b; return orig(b); }",
      );
      await click("button", "Save translated document");
      for (let i = 0; i < 80 && !(await evaluate("!!window.__saved")); i++)
        await sleep(100);
      const saved = await evaluate(
        `(async () => { const b = new Uint8Array(await window.__saved.arrayBuffer()); const r = window.interlineaCore.extract(b, 'en-GB'); const ex = JSON.parse(r.value); const u = Object.fromEntries(ex.units.map(x => [x.id, x.source.map(s => s.text ?? ('{' + s.tag.kind[0] + s.tag.id + '}')).join('')])); return [u['document/p1/s1'], u['document/p2/s1'], u['document/p25/s1'], u['document/p26/s1'], u['document/p20/s2']].join(' | '); })()`,
      );
      check(
        saved.startsWith(
          "Residential lease agreement | The following lease agreement is concluded between {o1}Mr Jonas Hofmann{c1}",
        ) &&
          saved.includes("Place, date | Place, date") &&
          saved.includes("Der Vermieter kann"),
        "saved .docx round-trips through the core: " + saved,
      );
      check(
        (await evaluate("document.body.innerText")).includes(
          "Saved Mietvertrag_Hofmann_2026_en-GB.docx",
        ),
        "save confirmed with the file name",
      );
      // Memories: the core's memories, TMX import into the project memory, export.
      await click("button", "Memories");
      await sleep(250);
      const memText = await evaluate(
        "document.querySelector('dialog[open]')?.innerText ?? ''",
      );
      check(
        /Kanzlei Weber \(DE-EN\)\s+lookup, concordance\s+13/.test(memText) &&
          memText.includes("project memory"),
        "memories dialog lists the client memory: " +
          memText.replace(/\s+/g, " ").slice(0, 120),
      );
      {
        const doc = await send("DOM.getDocument", {});
        const q = await send("DOM.querySelector", {
          nodeId: doc.root.nodeId,
          selector: "input[accept*='.tmx']",
        });
        await send("DOM.setFileInputFiles", {
          nodeId: q.nodeId,
          files: [`${fixtures}/test.tmx`],
        });
      }
      for (
        let i = 0;
        i < 40 &&
        !(await evaluate(
          "document.body.innerText.includes('Imported 1 unit')",
        ));
        i++
      )
        await sleep(100);
      check(
        (await evaluate("document.body.innerText")).includes(
          "Imported 1 unit from test.tmx; 1 without both languages skipped",
        ),
        "TMX imported through the core",
      );
      await evaluate("window.__saved = null");
      await evaluate(
        "[...document.querySelectorAll('dialog[open] tr')].find(r => r.textContent.includes('project memory')).querySelector('button:last-child').click()",
      );
      await sleep(300);
      const tmxOut = await evaluate(
        "window.__saved ? window.__saved.text() : ''",
      );
      check(
        tmxOut.includes("<tmx") &&
          tmxOut.includes("x-context-pre") &&
          tmxOut.includes("repaid after it ends"),
        "project memory exports as TMX with context",
      );
      await click("dialog[open] button", "Close");
      await sleep(150);
      await openSeg(21);
      check(
        (await evaluate(
          "document.querySelector('.result .score > [aria-hidden]')?.textContent",
        )) === "CM" &&
          (
            await evaluate(
              "document.querySelector('.result .result-meta')?.textContent",
            )
          ).includes("project memory"),
        "imported unit is a context match (Trados rule)",
      );
      // memoQ rule, set in the project's settings: the segment after must match too.
      await click("a", "Project settings");
      await sleep(500);
      await click(".choice", "before and after");
      await sleep(150);
      await click("button", "Save changes");
      await sleep(300);
      check(
        (await body()).includes("Saved the settings of “Mietvertrag Hofmann”"),
        "project settings saved",
      );
      await click("a", "Open in the editor");
      await sleep(800);
      await openSeg(21);
      check(
        (await evaluate(
          "document.querySelector('.result .score > [aria-hidden]')?.textContent",
        )) === "100%",
        "without the next segment it is 100% under the memoQ rule",
      );
      await openSeg(18);
      check(
        (await evaluate(
          "document.querySelector('.result .score > [aria-hidden]')?.textContent",
        )) === "CM",
        "both neighbours match: still CM",
      );
      await click("a", "Project settings");
      await sleep(500);
      await click(".choice", "The segment before matches");
      await sleep(150);
      await click("button", "Save changes");
      await sleep(300);
      await click("a", "Open in the editor");
      await sleep(500);
      await click("a", "Review and merge");
      await sleep(500);
      t = await text();
      check(
        t.includes("Vorauszahlung") && t.includes("advance payment"),
        "new term shows up in the merge review",
      );
      check(
        errors.length === 0,
        "no console errors in editor: " + errors.join(" | "),
      );

      // A real Word file chosen in New project goes through the Word filter.
      await open("/new", { wait: 0 });
      await sleep(500);
      await click("button", "Continue to files and languages");
      await sleep(300);
      {
        const doc = await send("DOM.getDocument", {});
        const q = await send("DOM.querySelector", {
          nodeId: doc.root.nodeId,
          selector: "input[type=file]",
        });
        await send("DOM.setFileInputFiles", {
          nodeId: q.nodeId,
          files: [`${samples}/Mietvertrag_Hofmann_2026.docx`],
        });
      }
      for (
        let i = 0;
        i < 80 &&
        !(await evaluate("document.body.innerText.includes('Detected')"));
        i++
      )
        await sleep(100);
      t = await text();
      check(
        t.includes("Detected German") &&
          t.includes("formatting kept by the Word filter"),
        "chosen .docx read by the core: " +
          (t.match(/Detected[^\n]*/)?.[0] ?? t.slice(0, 120)),
      );
      await evaluate(
        `(() => { const el = document.getElementById('client'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, 'Kanzlei Weber'); el.dispatchEvent(new Event('input', { bubbles: true })); })()`,
      );
      await sleep(200);
      await click("button", "Continue to memories");
      await sleep(300);
      await click("button", "Continue to terminology");
      await sleep(300);
      await click("button", "Continue to review");
      for (
        let i = 0;
        i < 80 && !(await evaluate("!!document.querySelector('.analysis')"));
        i++
      )
        await sleep(100);
      const an2 = await evaluate(
        "document.querySelector('.analysis')?.innerText ?? ''",
      );
      check(
        /Total\s+38/.test(an2) && /Context match\s+1/.test(an2),
        "review step shows the match analysis: " +
          an2.replace(/\s+/g, " ").slice(0, 140),
      );
      await click("button", "Create project");
      for (
        let i = 0;
        i < 80 && !(await evaluate("!!document.querySelector('.editor')"));
        i++
      )
        await sleep(100);
      check(
        (await evaluate("document.querySelectorAll('.seg').length")) === 38 &&
          (await evaluate("document.body.innerText")).includes(
            "Save translated document",
          ),
        "imported .docx opens with its units and can be saved",
      );
      check(
        errors.length === 0,
        "no console errors after import: " + errors.join(" | "),
      );

      // Merge review: resolve the conflict, merge, check the master (fresh state).
      await open("/merge/p-hofmann", { wait: 0 });
      for (
        let i = 0;
        i < 40 &&
        !(await evaluate("!!document.querySelector('.merge-bar button')").catch(
          () => false,
        ));
        i++
      )
        await sleep(100);
      check(
        await evaluate(`document.querySelector('.merge-bar button').disabled`),
        "merge blocked while conflict open",
      );
      await click(".decision label", "Keep both");
      await sleep(200);
      check(
        !(await evaluate(
          `document.querySelector('.merge-bar button').disabled`,
        )),
        "merge enabled after resolving",
      );
      await click(".change-new .decision input");
      await sleep(100); // leave out first new entry
      t = await text();
      check(
        t.includes("5 changes will be merged"),
        "count reflects one left out",
      );
      await click(".merge-bar button");
      await sleep(400);
      t = await text();
      check(
        t.includes("Merged 5 changes into Kanzlei Weber"),
        "merge done screen",
      );
      await click("a", "Open Kanzlei Weber");
      await sleep(400);
      t = await text();
      check(
        t.includes("Staffelmiete") && t.includes("Untervermietung"),
        "new entries in master",
      );
      // Conflict entry now has both English terms.
      await click(".entry-row", "Räumungsklage");
      await sleep(300);
      t = await text();
      check(
        t.includes("eviction action") && t.includes("action for possession"),
        "keep both merged synonyms",
      );
      await click("[role=tab]", "History");
      await sleep(200);
      t = await text();
      check(t.includes("kept both versions"), "history records the merge");

      // Back on the merge page (same page load): only the left-out entry remains.
      await open("/merge/p-hofmann", { wait: 0, reload: false });
      await sleep(600);
      t = await text();
      check(
        t.includes("New (1)") && t.includes("All (1)"),
        "only the left-out change remains: " + t.match(/All \(\d+\)/)?.[0],
      );

      // Git wording switch.
      await open("/settings", { wait: 0 });
      await sleep(400);
      await click(".choice", "Git terms");
      await sleep(200);
      await click("a", "Projects");
      await sleep(400);
      t = await evaluate("document.body.innerText");
      check(
        t.includes("Diff and merge") && t.includes("Branch"),
        "git wording applied",
      );

      // Add a field in the terminology manager.
      await open("/terminology/tb-mietrecht", { wait: 0 });
      await sleep(500);
      check(
        (await text()).includes("No entries yet"),
        "empty state for termbase without sample entries",
      );
    },
  );
