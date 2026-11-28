// Work survives a reload: projects with their Word file, translations,
// termbase entries, settings and imported memories; and the reset in
// Settings starts again from the sample data.
import { run } from "./harness.mjs";

const SAVE_DELAY = 900;

export default () =>
  run(
    "Saved work",
    async ({
      send,
      evaluate,
      click,
      text,
      body,
      check,
      sleep,
      open,
      setFile,
      setValue,
      key,
      fixtures,
      waitFor,
      startFrom,
      dlg,
    }) => {
      // A new project from a file.
      await startFrom(`${fixtures}/documents/Kaufvertrag_Mueller_Entwurf.txt`, { keep: false });
      await click("button", "Continue to files and languages");
      await sleep(300);
      await click("button", "Continue to memories");
      await sleep(300);
      await click("button", "Continue to terminology");
      await sleep(300);
      await click("button", "Continue to review");
      await sleep(200);
      await click("button", "Create project");
      await sleep(600);
      const projectHash = await evaluate("location.hash");
      check(
        projectHash.startsWith("#/editor/p-"),
        "project created: " + projectHash,
      );

      // A translation confirmed in the sample lease.
      await open("/editor/p-hofmann", { keep: true, wait: 2500 });
      await evaluate(
        "[...document.querySelectorAll('.seg')].find(s => s.querySelector('.seg-num').textContent === '9').querySelector('.seg-target-view, .target-editor').click()",
      );
      await sleep(400);
      await key("a", "KeyA", 65, 2);
      await send("Input.insertText", { text: "§ 3 Rent and service charges" });
      await sleep(200);
      await key("Enter", "Enter", 13, 2);
      await sleep(300);

      // A TMX file imported into the project memory, which already holds the
      // sample's confirmed segments and the one just confirmed.
      const projectUnits = () =>
        evaluate(
          "Number([...document.querySelectorAll('dialog[open] tr')].find(r => r.textContent.includes('project memory'))?.querySelector('td.num')?.textContent ?? -1)",
        );
      await click("button", "Memories");
      await sleep(250);
      const unitsBefore = await projectUnits();
      check(
        unitsBefore === 8,
        "project memory has the 7 sample units and the confirmed one: " +
          unitsBefore,
      );
      await setFile("input[accept*='.tmx']", `${fixtures}/test.tmx`);
      check(
        await waitFor("document.body.innerText.includes('Imported 1 unit')"),
        "TMX imported",
      );
      await evaluate("document.querySelector('dialog[open]').close()");

      // A termbase entry and a setting.
      await open("/terminology/tb-weber", { keep: true });
      await click(".new-entry-bar button", "New entry");
      await sleep(200);
      await setValue("#ne-text", "Persistenztest");
      await click(".new-entry button", "Add entry");
      await sleep(300);
      await open("/settings", { keep: true });
      const auto = await evaluate(
        "[...document.querySelectorAll('input[type=checkbox]')].find(i => i.closest('label').textContent.includes('Insert the best memory match')).checked",
      );
      await evaluate(
        "[...document.querySelectorAll('input[type=checkbox]')].find(i => i.closest('label').textContent.includes('Insert the best memory match')).click()",
      );
      await sleep(SAVE_DELAY);
      check(
        (await text()).includes("saved in this browser"),
        "Settings says work is saved",
      );

      // Reload without resetting: everything is still there.
      await open("/settings", { keep: true });
      check(
        (await evaluate(
          "[...document.querySelectorAll('input[type=checkbox]')].find(i => i.closest('label').textContent.includes('Insert the best memory match')).checked",
        )) === !auto,
        "setting kept",
      );
      await open("/projects", { keep: true });
      check(
        (await text()).includes("Kaufvertrag Mueller Entwurf"),
        "project kept",
      );
      await open(projectHash.slice(1), { keep: true, wait: 1500 });
      check(
        (await evaluate("document.querySelectorAll('.seg').length")) > 3,
        "its segments kept",
      );
      await open("/editor/p-hofmann", { keep: true, wait: 2500 });
      const seg9 = await evaluate(
        "(() => { const s = [...document.querySelectorAll('.seg')].find(s => s.querySelector('.seg-num').textContent === '9'); return s.querySelector('.seg-target-view, .target-editor').textContent + '|' + s.className; })()",
      );
      check(
        seg9.includes("§ 3 Rent and service charges") &&
          seg9.includes("confirmed"),
        "confirmed translation kept: " + seg9,
      );
      await click("button", "Memories");
      await sleep(400);
      const mem = await evaluate(
        "document.querySelector('dialog[open]')?.innerText ?? ''",
      );
      check(
        (await projectUnits()) === unitsBefore + 1,
        "imported memory kept: " + mem.replace(/\s+/g, " ").slice(0, 160),
      );
      await evaluate("document.querySelector('dialog[open]').close()");
      await open("/terminology/tb-weber", { keep: true });
      check((await text()).includes("Persistenztest"), "termbase entry kept");
      // The saved .docx can still be written from the kept Word file.
      await open("/editor/p-hofmann", { keep: true, wait: 2500 });
      await click("button", "Save translated document");
      check(
        await waitFor(
          "document.body.innerText.includes('Saved Mietvertrag_Hofmann_2026_en-GB.docx')",
        ),
        "Word file kept for saving",
      );

      // Reset from Settings.
      await open("/settings", { keep: true });
      await click("button", "Delete everything kept in this browser");
      await sleep(200);
      await click(".confirm button", "Delete everything");
      check(
        await waitFor(
          "document.body.innerText.includes('Everything kept in this browser was deleted')",
        ),
        "deleting announced",
      );
      await sleep(SAVE_DELAY);
      await open("/projects", { keep: true });
      let t = await text();
      check(
        !t.includes("Kaufvertrag Mueller Entwurf") &&
          !t.includes("Mietvertrag Hofmann") &&
          t.includes("No projects yet. Start one from files above."),
        "no project is left",
      );
      await open("/terminology", { keep: true });
      t = await text();
      check(
        t.includes("No termbase yet") && !t.includes("Kanzlei Weber"),
        "no termbase is left",
      );
      await open("/memories", { keep: true });
      check((await text()).includes("No memory yet."), "no memory is left");

      // A first visit starts with nothing: no sample, nothing called a server.
      await open("/projects", { sample: false });
      t = await text();
      check(
        t.includes("No projects yet. Start one from files above.") &&
          !/sample|prototype/i.test(t),
        "a first visit has no projects and offers no sample",
      );
      await open("/terminology", { keep: true });
      t = await text();
      check(
        t.includes("No termbase yet") && !t.includes("Server"),
        "a first visit has no termbases, and none said to be on a server",
      );
      // A master made there is on this computer.
      await click("button", "New master");
      check(
        await waitFor("(document.querySelector('dialog[open]')?.innerText ?? '').includes('New termbase')"),
        "a new master is made in a dialog, in steps",
      );
      check(
        !(await evaluate("!!document.querySelector('dialog[open] input[name=ntb-loc]')")),
        "without a team server there is no place to choose",
      );
      check(
        await evaluate("document.querySelector('dialog[open] input[name=ntb-kind]:not(:checked)')?.disabled === true"),
        "and a project termbase is made with its project",
      );
      await setValue("#ntb-name", "Erste Terminologie");
      await click("dialog[open] button", "Continue");
      check(
        await waitFor("document.querySelectorAll('dialog[open] .lang-pick-list label.check').length > 50"),
        "its languages are chosen from a list",
      );
      check(
        await evaluate("[...document.querySelectorAll('dialog[open] button')].find(b => b.textContent === 'Continue').disabled"),
        "and at least one is needed",
      );
      await setValue("dialog[open] .search-field input", "german");
      await sleep(100);
      check(
        await evaluate("[...document.querySelectorAll('dialog[open] .lang-pick-list label.check')].every(l => /German/.test(l.textContent))"),
        "the list is searched by name",
      );
      await evaluate("[...document.querySelectorAll('dialog[open] .lang-pick-list label.check')].find(l => l.textContent.includes('de-DE')).querySelector('input').click()");
      await setValue("dialog[open] .search-field input", "en-gb");
      await sleep(100);
      await evaluate("[...document.querySelectorAll('dialog[open] .lang-pick-list label.check')].find(l => l.textContent.includes('en-GB')).querySelector('input').click()");
      await sleep(100);
      check(
        (await dlg()).includes("German (Germany) ×") && (await dlg()).includes("English (United Kingdom) ×"),
        "and by code, and what is chosen is shown",
      );
      await click("dialog[open] button", "Continue");
      await waitFor("!!document.querySelector('dialog[open] input[name=ntb-group]')");
      const rows = () => evaluate("document.querySelectorAll('dialog[open] .field-table tbody tr').length");
      const group = (id) => evaluate(`document.querySelector('dialog[open] input[name=ntb-group][value=${id}]').click(), true`);
      // Groups of fields, in any combination.
      await group("definitions");
      await group("grammar");
      check(await waitFor("document.querySelectorAll('dialog[open] .field-table tbody tr').length === 5"), "definitions and grammar together give their fields: " + (await rows()));
      await group("definitions");
      check(
        await waitFor("document.querySelectorAll('dialog[open] .field-table tbody tr').length === 3 && !document.querySelector('dialog[open] input[name=ntb-group][value=definitions]').checked"),
        "a group taken away takes its fields with it, and leaves the others",
      );
      for (const id of ["definitions", "note", "subject", "usage", "checked"]) await group(id);
      check(
        await waitFor("document.querySelectorAll('dialog[open] .field-table tbody tr').length === 10 && [...document.querySelectorAll('dialog[open] input[name=ntb-group]')].every(i => i.checked)"),
        "all the groups give all their fields: " + (await rows()),
      );
      check(
        await evaluate("[...document.querySelectorAll('dialog[open] .field-table tbody tr')].find(r => r.querySelector('input').value === 'Gender')?.querySelector('textarea')?.value.split('\\n').length === 4"),
        "with the values of its lists",
      );
      // A field of one's own: a name is needed, and one not taken.
      await click("dialog[open] .field-table button", "Add a field");
      await sleep(100);
      check((await dlg()).includes("Give the field a name."), "a field needs a name");
      await setValue("dialog[open] .field-table tbody tr:last-child input", "Gender");
      await sleep(100);
      check((await dlg()).includes("Another field has this name."), "and one no other field has");
      await setValue("dialog[open] .field-table tbody tr:last-child input", "Checked by");
      await sleep(100);
      await click("dialog[open] button", "Continue");
      await sleep(150);
      const review = await dlg();
      check(
        review.includes("German (Germany), English (United Kingdom)") && review.includes("Gender (term, a list of values: masculine, feminine, neuter, common)") && review.includes("Checked by (entry, text)"),
        "what will be made is shown before it is made",
      );
      await click("dialog[open] button", "Create master");
      check(
        await waitFor("document.body.innerText.includes('Created master')"),
        "the first master is made",
      );
      t = await text();
      check(
        t.includes("Erste Terminologie") && t.includes("No entries yet"),
        "and opens, empty",
      );
      // The sample is added on request, beside what is there.
      await sleep(SAVE_DELAY);
      await evaluate("location.href = location.origin + '/?sample=1#/terminology'");
      check(
        await waitFor("document.body.innerText.includes('Kanzlei Weber')", 15000),
        "the sample is added when the address asks for it",
      );
      check(
        (await text()).includes("Erste Terminologie"),
        "and what was there stays",
      );
    },
  );
