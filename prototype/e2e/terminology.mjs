// Terminology, import, export and term recognition
import { run } from "./harness.mjs";

export default () =>
  run(
    "Terminology, import, export and term recognition",
    async ({
      send,
      evaluate,
      click,
      text,
      body,
      dlg,
      check,
      sleep,
      open,
      setFile,
      setValue,
      key,
      errors,
      fixtures,
      downloads,
    }) => {
      // Term recognition and checks in the editor run in the core.
      await open("/editor/p-hofmann", { wait: 0 });
      await sleep(2500);
      check(
        (await evaluate(
          "[...document.querySelectorAll('.seg.is-active .term-hit')].map(h => h.textContent).join('|')",
        )) === "ordentliche Kündigung",
        "longest term wins: " +
          (await evaluate(
            "[...document.querySelectorAll('.seg.is-active .term-hit')].map(h => h.textContent).join('|')",
          )),
      );
      await evaluate(
        "[...document.querySelectorAll('.seg')].find(s => s.querySelector('.seg-num').textContent === '9').querySelector('.seg-target-view, .target-editor').click()",
      );
      await sleep(400);
      await key("a", "KeyA", 65, 2);
      await send("Input.insertText", { text: "§ 3 Rent and operating costs" });
      await sleep(400);
      let iss = await evaluate(
        "[...document.querySelectorAll('.issue')].map(i => i.textContent).join(' | ')",
      );
      check(
        iss.includes(
          "“operating costs” is a forbidden term for “Betriebskosten”",
        ) && iss.includes("“Betriebskosten” is usually “service charges”"),
        "forbidden and missing terms flagged: " + iss,
      );
      await key("a", "KeyA", 65, 2);
      await send("Input.insertText", { text: "§ 3 Rent and service charges" });
      await sleep(400);
      iss = await evaluate(
        "[...document.querySelectorAll('.issue')].map(i => i.textContent).join(' | ')",
      );
      check(
        !iss.includes("Betriebskosten"),
        "correct term clears the checks: " + iss,
      );

      await open("/terminology/tb-weber", { wait: 0 });
      await sleep(1500);
      const before = await evaluate(
        "document.querySelectorAll('.entry-list li').length",
      );

      // Import a CSV glossary into Kanzlei Weber.
      await click(".pane-tools button", "Import");
      await sleep(1500);
      await setFile("dialog input[type=file]", `${fixtures}/glossar.csv`);
      await sleep(500);
      let d = await dlg();
      check(
        d.includes("Columns") &&
          d.includes("Deutsch") &&
          d.includes("Status Englisch"),
        "CSV opens the column step",
      );
      check(
        (await evaluate("document.querySelector('#col-3-role').value")) ===
          "status" &&
          (await evaluate("document.querySelector('#col-0-role').value")) ===
            "id",
        "ID and status columns recognised",
      );
      await click("dialog button", "Continue");
      await sleep(300);
      d = await dlg();
      check(
        d.includes("4 entries, 9 terms"),
        "read report: " + d.match(/CSV[^\n]*/)?.[0],
      );
      check(
        (await evaluate("document.querySelector('#lm-de').value")) ===
          "de-DE" &&
          (await evaluate("document.querySelector('#lm-en').value")) ===
            "en-GB",
        "languages mapped to the termbase's variants",
      );
      check(d.includes("New fields: Quelle"), "new field announced");
      await click("dialog button", "Continue");
      await sleep(400);
      d = await dlg();
      check(
        /New entries\s*3/.test(d) && /Merged\s*1/.test(d),
        "plan: 3 new, 1 merged: " +
          d.match(/New entries[\s\S]{0,60}/)?.[0].replace(/\s+/g, " "),
      );
      await click("dialog label.check", "Keep it");
      await sleep(300);
      d = await dlg();
      check(/Unchanged\s*1/.test(d), "skip policy changes the plan");
      await click("dialog label.check", "Merge:");
      await sleep(300);
      await click("dialog button", "Import");
      await sleep(600);
      let t = await body();
      check(t.includes("3 added, 1 merged"), "toast reports the import");
      check(
        (await evaluate(
          "document.querySelectorAll('.entry-list li').length",
        )) ===
          before + 3,
        "three entries added",
      );
      await click(".entry-row", "Kaution");
      await sleep(300);
      t = await text();
      check(
        t.includes("rental deposit") &&
          t.includes("security deposit") &&
          t.includes("Quelle"),
        "merged entry has the new term and field",
      );
      await click("[role=tab]", "History");
      await sleep(200);
      check(
        (await text()).includes("Merged with an entry from glossar.csv"),
        "history records the merge",
      );
      await click("[role=tab]", "Entry");
      await sleep(200);

      // Edit terms.
      await click(
        ".lang-block[aria-label='English (United Kingdom)'] button",
        "Add a synonym",
      );
      await sleep(200);
      await setValue(
        ".lang-block[aria-label='English (United Kingdom)'] .term-edit input",
        "bond",
      );
      await click(
        ".lang-block[aria-label='English (United Kingdom)'] .term-edit button",
        "Add",
      );
      await sleep(300);
      t = await text();
      check(t.includes("bond"), "synonym added");
      await evaluate(
        `(() => { const li = [...document.querySelectorAll('.term')].find(l => l.querySelector('.term-text')?.textContent === 'bond'); const s = li.querySelector('select'); Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s, 'forbidden'); s.dispatchEvent(new Event('change', { bubbles: true })); })()`,
      );
      await sleep(300);
      check(
        (await evaluate(
          `[...document.querySelectorAll('.term')].find(l => l.querySelector('.term-text')?.textContent === 'bond').querySelector('.status').textContent`,
        )) === "Forbidden",
        "status changed",
      );
      await evaluate(
        `[...document.querySelectorAll('.term')].find(l => l.querySelector('.term-text')?.textContent === 'bond').querySelector('.confirm, .term-tools .btn-quiet').click()`,
      );
      await sleep(200);
      await click(".term .confirm button", "Remove");
      await sleep(300);
      check(!(await text()).includes("bond"), "term removed after confirming");

      // Duplicates: add a second "Mietsache" entry and merge.
      await click(".new-entry-bar button", "New entry");
      await sleep(200);
      await setValue("#ne-text", "Mietsache");
      await click(".new-entry button", "Add entry");
      await sleep(400);
      await click("[role=tab]", "Duplicates");
      await sleep(1200);
      t = await text();
      check(
        t.includes("Merge into one entry") && t.includes("Mietsache"),
        "duplicate group found",
      );
      const n1 = await evaluate(
        "document.querySelectorAll('.dup-group').length",
      );
      await click(".dup-group button", "Merge into one entry");
      await sleep(400);
      check(
        (await evaluate("document.querySelectorAll('.dup-group').length")) ===
          n1 - 1,
        "group merged",
      );
      await click("[role=tab]", "Entries");
      await sleep(300);
      check(
        (await evaluate(
          "[...document.querySelectorAll('.entry-term')].filter(e => e.textContent === 'Mietsache').length",
        )) === 1,
        "one Mietsache entry left",
      );

      // Export TBX and Excel.
      await click(".pane-tools button", "Export");
      await sleep(400);
      await click("dialog button", "Export");
      await sleep(800);
      await click(".pane-tools button", "Export");
      await sleep(300);
      await click("dialog label.choice", "Excel workbook");
      await sleep(100);
      await click("dialog button.btn-primary", "Export");
      await sleep(800);

      const fs = await import("node:fs");
      const files = fs.readdirSync(downloads);
      check(
        files.includes("Kanzlei_Weber.tbx") &&
          files.includes("Kanzlei_Weber.xlsx"),
        "exports downloaded: " + files.join(", "),
      );
      if (files.includes("Kanzlei_Weber.tbx")) {
        const x = fs.readFileSync(`${downloads}/Kanzlei_Weber.tbx`, "utf8");
        check(
          x.includes("<term>Untermiete</term>") &&
            x.includes('id="K-2"') &&
            x.includes("rental deposit"),
          "TBX holds the imported entries with their IDs",
        );
      }

      // Import the TBX file as a new termbase.
      await click(".tb-actions button", "Import termbase");
      await sleep(400);
      await setFile("dialog input[type=file]", `${fixtures}/glossar.tbx`);
      await sleep(500);
      d = await dlg();
      check(
        d.includes("TBX 3: 4 entries, 9 terms") && !d.includes("Columns"),
        "TBX skips the column step",
      );
      await click("dialog button", "Continue");
      await sleep(400);
      d = await dlg();
      check(
        /New entries\s*4/.test(d) && !d.includes("Find existing entries"),
        "new termbase: everything added",
      );
      await click("dialog button", "Import");
      await sleep(600);
      t = await body();
      check(
        t.includes("glossar") &&
          (await evaluate(
            "document.querySelectorAll('.entry-list li').length",
          )) === 4,
        "new termbase with 4 entries",
      );
      check(
        await evaluate("location.hash").then((h) =>
          h.startsWith("#/terminology/tb-"),
        ),
        "navigated to the new termbase",
      );

      // Settings: rename, add a language, remove a field.
      await click(".pane-tools button", "Settings");
      await sleep(300);
      await setValue("#tb-name", "Glossar Mandant");
      await setValue("#tbs-add-lang", "fr-FR");
      await click("dialog button", "Add");
      await sleep(200);
      d = await dlg();
      check(d.includes("French (France)"), "language added");
      await evaluate(
        "[...document.querySelectorAll('dialog tr')].find(r => r.querySelector('input')?.value === 'Quelle').querySelector('.btn-quiet').click()",
      );
      await sleep(200);
      await click("dialog .confirm button", "Remove");
      await sleep(200);
      check(
        !(await evaluate(
          "[...document.querySelectorAll('dialog tr input')].some(i => i.value === 'Quelle')",
        )),
        "field removed",
      );
      await click("dialog button", "Done");
      await sleep(300);
      t = await text();
      check(t.includes("Glossar Mandant"), "renamed");

      // Errors.
      await click(".pane-tools button", "Import");
      await sleep(300);
      await setFile("dialog input[type=file]", `${fixtures}/test.tmx`);
      await sleep(300);
      check(
        (await dlg()).includes("could not be read"),
        "a TMX file is refused with a reason: " +
          (await dlg()).match(/could not[^\n]*/)?.[0],
      );
      await evaluate(
        "document.querySelector('dialog[open]').dispatchEvent(new Event('cancel'))",
      );
      await sleep(200);
      check(
        !(await evaluate("!!document.querySelector('dialog[open]')")),
        "Escape closes the dialog",
      );
    },
  );
