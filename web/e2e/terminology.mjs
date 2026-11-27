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
      waitFor,
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
      check((await dlg()).includes("To read and print") && (await dlg()).includes("To exchange with other tools"), "the export offers glossaries to read and exchange formats");
      await click("dialog label.choice", "TBX 3");
      await click("dialog button.btn-primary", "Export");
      await sleep(800);
      await click(".pane-tools button", "Export");
      await sleep(300);
      await click("dialog label.choice", "XLSX workbook");
      await sleep(100);
      await click("dialog button.btn-primary", "Export");
      await sleep(800);
      // A printable dictionary, made in the browser with the app's fonts.
      await click(".pane-tools button", "Export");
      await sleep(300);
      await click("dialog label.choice", ".pdf");
      await click("dialog button", "Continue");
      await sleep(100);
      check((await dlg()).includes("Languages") && (await dlg()).includes("Fields"), "the content is chosen");
      await click("dialog button", "Continue");
      await sleep(100);
      check((await dlg()).includes("Dictionary") && (await dlg()).includes("Sorted by"), "and the layout");
      await click("dialog button.btn-primary", "Export to .pdf");
      check(await waitFor("[...document.querySelectorAll('.toast')].some(t => t.textContent.includes('Kanzlei_Weber.pdf'))", 20000), "the PDF is made");
      // A table in Word, German and English only, kept as settings of the termbase.
      await click(".pane-tools button", "Export");
      await sleep(300);
      await click("dialog label.choice", ".docx");
      await click("dialog button", "Continue");
      await sleep(100);
      await evaluate("[...document.querySelectorAll('dialog [aria-label=\"Languages exported\"] label.check')].filter(l => !/de-DE|en-GB/.test(l.textContent)).forEach(l => l.querySelector('input').click())");
      await click("dialog button", "Continue");
      await sleep(100);
      await click("dialog label.choice", "Table");
      await setValue("#exp-keep", "Client table");
      await click("dialog button", "Keep");
      check(await waitFor("[...document.querySelectorAll('.toast')].some(t => t.textContent.includes('Kept the export settings'))"), "export settings are kept with the termbase");
      await click("dialog button.btn-primary", "Export to .docx");
      await sleep(800);
      await click(".pane-tools button", "Export");
      await sleep(300);
      check(await evaluate("[...document.querySelectorAll('#exp-preset option')].some(o => o.textContent === 'Client table') && [...document.querySelectorAll('#exp-preset option')].some(o => o.textContent === 'Printable dictionary')"), "kept and ready-made settings are offered");
      await setValue("#exp-preset", "Web page");
      await sleep(100);
      await click("dialog button.btn-primary", "Export to .html");
      await sleep(800);
      const fs = await import("node:fs");
      const files = fs.readdirSync(downloads);
      check(
        ["tbx", "xlsx", "pdf", "docx", "html"].every((x) => files.includes(`Kanzlei_Weber.${x}`)),
        "exports downloaded: " + files.join(", "),
      );
      if (files.includes("Kanzlei_Weber.pdf")) {
        const pdf = fs.readFileSync(`${downloads}/Kanzlei_Weber.pdf`, "latin1");
        check(pdf.startsWith("%PDF-1.7") && pdf.includes("/FontFile2") && pdf.includes("/Outlines"), "the PDF has its fonts and its letters as bookmarks");
      }
      if (files.includes("Kanzlei_Weber.html")) {
        const html = fs.readFileSync(`${downloads}/Kanzlei_Weber.html`, "utf8");
        check(html.includes("<!DOCTYPE html>") && html.includes("Mietsache") && html.includes('id="find"'), "the web page has the entries and a search");
      }
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

      // A MultiTerm termbase file is read as it is, without an export.
      await click(".tb-actions button", "Import termbase");
      await sleep(400);
      await setFile("dialog input[type=file]", `${fixtures}/orchard.sdltb`);
      await sleep(700);
      d = await dlg();
      check(
        d.includes(".sdltb termbase: 5 entries, 15 terms") &&
          !d.includes("Columns"),
        "a MultiTerm termbase file is read",
      );
      await click("dialog button", "Continue");
      await sleep(400);
      d = await dlg();
      check(/New entries\s*5/.test(d), "its five entries are new");
      await click("dialog button", "Import");
      await sleep(700);
      t = await body();
      check(
        t.includes("Orchard") &&
          (await evaluate(
            "document.querySelectorAll('.entry-list li').length",
          )) === 5,
        "the termbase takes its name and entries from the file",
      );
      check(
        t.includes("りんご") && t.includes("تفاحة") && t.includes("μήλο"),
        "terms in other scripts come through",
      );
      await evaluate("history.back()");
      await sleep(500);

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

      // A termbase kept in this browser has what one on the server has:
      // versions to restore, deleted entries to bring back, the master's
      // entries under a project termbase, and review rules.
      const toast = (part) =>
        waitFor(
          `[...document.querySelectorAll('.toast')].some(t => t.textContent.includes(${JSON.stringify(part)}))`,
          5000,
        );
      const tab = (label) =>
        evaluate(
          `[...document.querySelectorAll('[role=tab]')].find(t => t.textContent.trim().startsWith(${JSON.stringify(label)})).click()`,
        );
      const rows = () =>
        evaluate(
          "[...document.querySelectorAll('.pane-entries .entry-row')].map(r => r.querySelector('.entry-term').textContent).join('|')",
        );
      await open("/terminology/tb-weber", { wait: 0 });
      await waitFor("!!document.querySelector('.entry-row')");
      await click(".entry-row", "Mietsache");
      await sleep(300);
      const badge = () =>
        evaluate(
          "[...document.querySelectorAll('.pane-detail .term')].find(t => t.querySelector('.term-text').textContent === 'rented property').querySelector('.badge, [class*=status-]').textContent.trim()",
        );
      check((await badge()) === "Proposed", "the term starts as proposed: " + (await badge()));
      await evaluate(
        "(() => { const s = [...document.querySelectorAll('.pane-detail .term')].find(t => t.querySelector('.term-text').textContent === 'rented property').querySelector('select'); Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s, 'approved'); s.dispatchEvent(new Event('change', { bubbles: true })); })()",
      );
      await sleep(300);
      check((await badge()) === "Approved", "the status is changed: " + (await badge()));
      await tab("History");
      await sleep(300);
      let hist = await evaluate(
        "[...document.querySelectorAll('.history li')].map(li => (li.querySelector('.history-held')?.textContent ?? '-') + ' / ' + (li.querySelector('button') ? 'restore' : 'none')).join(' | ')",
      );
      check(
        hist.split(" | ")[0].includes("rented property") &&
          hist.split(" | ")[0].endsWith("none") &&
          hist.split(" | ")[1].includes("rented property") &&
          hist.split(" | ")[1].endsWith("restore"),
        "the history shows what each version held and restores an earlier one: " + hist,
      );
      await click(".history button", "Restore this version");
      await sleep(200);
      check(
        (await text()).includes("What it is now stays in the history"),
        "restoring asks first",
      );
      await click(".history .confirm button", "Restore");
      check(await toast("Restored the version of"), "restoring is said");
      check(
        (await evaluate("document.querySelector('.history li .history-summary').textContent")).startsWith("Restored the version of"),
        "the restore is a version of its own",
      );
      await tab("Entry");
      await sleep(300);
      check((await badge()) === "Proposed", "the earlier version is back: " + (await badge()));

      // Deleted entries are kept and come back with their history.
      await click(".pane-detail button", "Delete entry");
      await sleep(200);
      check(
        (await text()).includes("It can be brought back from Deleted."),
        "deleting says the entry can come back",
      );
      await click(".pane-detail .confirm button", "Delete");
      await sleep(400);
      check(!(await rows()).split("|").includes("Mietsache"), "the entry is gone from the list");
      await tab("Deleted");
      await sleep(300);
      t = await evaluate("document.querySelector('.deleted-list')?.innerText ?? ''");
      check(
        t.includes("Mietsache") && t.includes("leased property") && t.includes("Deleted "),
        "the deleted entry is listed with its terms: " + t.replace(/\n/g, " "),
      );
      // It is still there after the page is loaded again.
      await open("/terminology/tb-weber", { wait: 0, keep: true });
      await waitFor("!!document.querySelector('.entry-row')");
      await tab("Deleted");
      await sleep(300);
      check(
        (await evaluate("document.querySelector('.deleted-list')?.innerText ?? ''")).includes("Mietsache"),
        "deleted entries are kept across loads",
      );
      await click(".deleted-entry button", "Bring back");
      check(await toast("Brought back “Mietsache”"), "bringing back is said");
      check(
        (await text()).includes("Nothing deleted"),
        "nothing is left under Deleted",
      );
      await tab("Entries");
      await sleep(300);
      check((await rows()).split("|").includes("Mietsache"), "the entry is back in the list");
      await click(".entry-row", "Mietsache");
      await sleep(200);
      await tab("History");
      await sleep(200);
      hist = await evaluate("[...document.querySelectorAll('.history .history-summary')].map(x => x.textContent).join(' | ')");
      check(
        hist.startsWith("Brought back") && hist.includes("Created entry"),
        "it comes back with its history: " + hist,
      );

      // Review rules of a master kept in this browser.
      await click(".pane-tools button", "Settings");
      await sleep(300);
      d = await dlg();
      check(
        d.includes("Review") &&
          d.includes("Entries the master already has as they are need no review") &&
          d.includes("Fields that need no approval") &&
          d.includes("Trusted authors") &&
          d.includes("Nobody yet: every entry waits for review."),
        "a master in the browser has review rules",
      );
      await evaluate(
        "[...document.querySelectorAll('dialog [aria-label=\"Fields that need no approval\"] label')].find(l => l.textContent.trim() === 'Definition').querySelector('input').click()",
      );
      check(await toast("Changes to Definition only are merged without review"), "a field that needs no approval is set");
      await click("dialog form[aria-label='Trust an author'] button", "Trust");
      await sleep(200);
      check((await dlg()).includes("Say whose entries to trust."), "trusting nobody is refused");
      await evaluate(
        "(() => { const s = document.querySelector('dialog form[aria-label=\"Trust an author\"] select'); const o = [...s.options].find(o => o.textContent === 'Anna Lindner'); Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s, o.value); s.dispatchEvent(new Event('change', { bubbles: true })); })()",
      );
      await click("dialog form[aria-label='Trust an author'] button", "Trust");
      check(await toast("The entries of Anna Lindner need no review"), "an author is trusted");
      check(
        (await evaluate("document.querySelector('dialog .mr-reviewers')?.innerText ?? ''")).replace(/\s+/g, " ").includes("Anna Lindner all entries"),
        "the trusted author is listed",
      );
      await click("dialog button", "Done");
      await sleep(300);

      // The merge review lists apart what the rules cover; a conflict always waits.
      await open("/merge/p-hofmann", { wait: 0, reload: false });
      await waitFor("!!document.querySelector('.merge-bar button')");
      t = await evaluate("document.querySelector('.changes-covered')?.innerText ?? ''");
      check(
        /Need no review \(5\)/.test(t) &&
          t.includes("By Anna Lindner, whose entries need no review.") &&
          t.includes("The rules of Kanzlei Weber cover these."),
        "what the rules cover is listed apart: " + t.slice(0, 120).replace(/\n/g, " "),
      );
      check(
        await evaluate(
          "!document.querySelector('.changes-covered .change-conflict') && document.querySelectorAll('main > .changes .change-conflict, .changes:not(.changes-covered .changes) .change-conflict').length === 1",
        ),
        "the conflict still waits for review",
      );

      // The trust is taken back: everything waits again.
      await open("/terminology/tb-weber", { wait: 0, reload: false });
      await waitFor("!!document.querySelector('.entry-row')");
      await click(".pane-tools button", "Settings");
      await sleep(300);
      await click("dialog .mr-reviewers button", "Remove");
      check(await toast("The entries of Anna Lindner wait for review again"), "the trust is taken back");
      await click("dialog button", "Done");
      await sleep(200);

      // Under a project termbase: the master's entries it uses as they are.
      await open("/terminology/tb-p-weber-lease", { wait: 0, reload: false });
      await waitFor("!!document.querySelector('.entry-row')");
      const masterTab = () =>
        evaluate("[...document.querySelectorAll('[role=tab]')].find(t => t.textContent.trim().startsWith('Master'))?.textContent.trim() ?? ''");
      const count = Number(/\((\d+)\)/.exec(await masterTab())?.[1] ?? -1);
      check(count > 1, "a project termbase in the browser has the Master view: " + (await masterTab()));
      await tab("Master");
      await sleep(300);
      let ms = await rows();
      check(
        ms.split("|").length === count &&
          ms.split("|").includes("Mietsache") &&
          !ms.split("|").includes("Kaution") &&
          !ms.split("|").includes("Räumungsklage"),
        "it lists the master's entries the project has no version of: " + ms,
      );
      check(
        (await text()).includes("The master's entries this project uses as they are. Change one for this project only, or leave it out."),
        "the view says what it is",
      );
      await click(".entry-row", "Mietsache");
      await sleep(200);
      t = await evaluate("document.querySelector('.pane-detail').innerText");
      check(
        t.includes("In Kanzlei Weber") && t.includes("This is the master's entry.") && t.includes("leased property"),
        "a master's entry is read there",
      );
      await click(".pane-detail button", "Leave out of this project");
      await sleep(200);
      await click(".pane-detail .confirm button", "Leave out");
      check(await toast("Left the entry out of the project"), "leaving out is said");
      ms = await rows();
      check(!ms.split("|").includes("Mietsache") && (await masterTab()) === `Master (${count - 1})`, "the entry left out is no longer among them: " + (await masterTab()));
      t = await evaluate("document.querySelector('.left-out')?.innerText ?? ''");
      check(
        t.includes("Left out of this project") && t.includes("Mietsache") && t.includes("In Kanzlei Weber, left out"),
        "and is listed as left out: " + t.replace(/\n/g, " "),
      );

      // The merge offers to delete it in the master.
      await open("/merge/p-hofmann", { wait: 0, reload: false });
      await waitFor("!!document.querySelector('.merge-bar button')");
      t = await evaluate("document.querySelector('.change-removed')?.innerText ?? ''");
      check(
        t.includes("Mietsache") &&
          t.includes("Left out of the project") &&
          t.includes("Accepting deletes the entry in Kanzlei Weber") &&
          t.includes("Delete in Kanzlei Weber"),
        "the merge review shows the entry left out: " + t.replace(/\n/g, " "),
      );
      check(
        (await evaluate("document.querySelector('.segmented').innerText")).includes("Left out (1)"),
        "and counts it",
      );

      // Brought back, it is used again; taken into the project, it is changed there.
      await open("/terminology/tb-p-weber-lease", { wait: 0, reload: false });
      await waitFor("!!document.querySelector('.entry-row')");
      await tab("Master");
      await sleep(300);
      await click(".left-out button", "Bring back");
      check(await toast("Brought the entry back into the project"), "bringing back into the project is said");
      check(
        (await rows()).split("|").includes("Mietsache") && !(await evaluate("!!document.querySelector('.left-out')")),
        "the entry is used again",
      );
      await click(".entry-row", "Mietsache");
      await sleep(200);
      await click(".pane-detail button", "Change in this project");
      check(await toast("Took the entry into the project"), "taking into the project is said");
      await sleep(300);
      check(
        (await evaluate("document.querySelector('[role=tab][aria-selected=true]').textContent.trim()")) === "Entries" &&
          (await evaluate("document.querySelector('#detail-title')?.textContent")) === "Mietsache" &&
          (await evaluate("!!document.querySelector('.pane-detail select')")) &&
          (await rows()).split("|").includes("Mietsache"),
        "the project's version opens to be changed",
      );
      check((await masterTab()) === `Master (${count - 1})`, "the master's own is no longer listed: " + (await masterTab()));

      // An entry left out and merged is deleted in the master, and kept under Deleted there.
      await tab("Master");
      await sleep(300);
      const victim = (await rows()).split("|")[0];
      await click(".entry-row", victim);
      await sleep(200);
      await click(".pane-detail button", "Leave out of this project");
      await sleep(200);
      await click(".pane-detail .confirm button", "Leave out");
      await sleep(300);
      await open("/merge/p-hofmann", { wait: 0, reload: false });
      await waitFor("!!document.querySelector('.merge-bar button')");
      await click(".decision label", "Keep both");
      await sleep(200);
      await click(".merge-bar button");
      await sleep(500);
      await open("/terminology/tb-weber", { wait: 0, reload: false });
      await waitFor("!!document.querySelector('.entry-row')");
      check(!(await rows()).split("|").includes(victim), `the merge deleted “${victim}” in the master`);
      await tab("Deleted");
      await sleep(300);
      t = await evaluate("document.querySelector('.deleted-list')?.innerText ?? ''");
      check(t.includes(victim) && t.includes("Deleted by a merge"), "it is kept under Deleted there: " + t.replace(/\n/g, " "));
      await open("/terminology/tb-p-weber-lease", { wait: 0, reload: false });
      await waitFor("!!document.querySelector('.entry-row')");
      await tab("Master");
      await sleep(300);
      check(!(await evaluate("!!document.querySelector('.left-out')")), "nothing is left out any more");

      // Fields of each kind, added from an entry, take values with the control their kind asks for.
      await open("/terminology/tb-weber", { wait: 0, reload: false });
      await waitFor("!!document.querySelector('.entry-row')");
      await click(".entry-row");
      await waitFor("!!document.querySelector('#panel-entry')");
      const addField = async (name, kind, values) => {
        await click("#panel-entry button", "Add a field");
        await waitFor("!!document.querySelector('#panel-entry form.add-field')");
        await setValue("#panel-entry form.add-field input:not([type=radio])", name);
        await evaluate("document.querySelector('#panel-entry form.add-field input[type=radio]').click()");
        await setValue("#panel-entry form.add-field select", kind);
        if (values) await setValue("#panel-entry form.add-field textarea", values);
        await sleep(100);
        await click("#panel-entry form.add-field button", "Add field");
        return toast(`Added field “${name}”`);
      };
      const control = (name) => `[...document.querySelectorAll('#panel-entry .field-row')].find(r => r.querySelector('label')?.textContent === ${JSON.stringify(name)})?.querySelector('dd > *')`;
      check(await addField("Geprüft", "boolean"), "a yes-or-no field is added");
      check(
        await waitFor(`${control("Geprüft")}?.tagName === 'SELECT' && [...${control("Geprüft")}.options].map(o => o.text).join() === 'Not set,Yes,No'`),
        "it is answered with yes or no",
      );
      await evaluate(`(() => { const el = ${control("Geprüft")}; Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(el, 'true'); el.dispatchEvent(new Event('change', { bubbles: true })); return true; })()`);
      await sleep(200);
      check(await evaluate(`${control("Geprüft")}.value === 'true'`), "and keeps its answer");
      check(await addField("Stand", "date"), "a date field is added");
      check(await waitFor(`${control("Stand")}?.type === 'date'`), "it takes a date from a calendar");
      check(await addField("Register", "picklist", "formal\ninformal"), "a list field is added with its values, one a line");
      check(
        await waitFor(`[...(${control("Register")}?.options ?? [])].map(o => o.text).join() === 'Not set,formal,informal'`),
        "its values are offered to choose from",
      );
      check(await addField("Seiten", "number"), "a number field is added");
      await waitFor(`!!${control("Seiten")}?.querySelector('input')`);
      await evaluate(`(() => { const el = ${control("Seiten")}.querySelector('input'); el.focus(); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, 'zwölf'); el.dispatchEvent(new Event('input', { bubbles: true })); return true; })()`);
      await sleep(100);
      await evaluate(`${control("Seiten")}.querySelector('input').dispatchEvent(new FocusEvent('focusout', { bubbles: true }))`);
      check(await waitFor(`${control("Seiten")}.innerText.includes('Type a number, such as 12 or 0.5.')`), "a number field refuses what is no number");
      // A field added from an entry is removed from it again, with its values.
      const row = (name) => `[...document.querySelectorAll('#panel-entry .field-row')].find(r => r.querySelector('label')?.textContent === ${JSON.stringify(name)})`;
      await evaluate(`${row("Geprüft")}.querySelector('dd > button:last-child').click()`);
      check(
        await waitFor(`${row("Geprüft")}?.innerText.includes('Remove “Geprüft” and 1 value?')`),
        "removing a field from the entry says how many values go with it: " + (await evaluate(`${row("Geprüft")}?.innerText ?? 'no row'`)),
      );
      await evaluate(`[...${row("Geprüft")}.querySelectorAll('.confirm button')].find(b => b.textContent.trim() === 'Remove').click()`);
      check(await toast("Removed the field “Geprüft”"), "and says it is removed");
      check(await waitFor(`!${row("Geprüft")}`), "the field is gone from the entry");
      await click(".pane-tools button", "Settings");
      check(
        await waitFor("[...document.querySelectorAll('dialog[open] tr')].find(r => r.querySelector('input')?.value === 'Register')?.querySelector('textarea')?.value === 'formal\\ninformal'"),
        "the settings show a list's values one a line",
      );
      check(
        await evaluate("[...[...document.querySelectorAll('dialog[open] tr')].find(r => r.querySelector('input')?.value === 'Stand').querySelector('select').options].map(o => o.text).length === 5"),
        "and offer the five kinds of value",
      );
      await click("dialog[open] button", "Done");
      check(errors.length === 0, "no console errors: " + errors.join("; "));
    },
  );
