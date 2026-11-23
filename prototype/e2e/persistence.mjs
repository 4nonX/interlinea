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
    }) => {
      // A new project from a sample document.
      await open("/projects");
      await click("button", "Kaufvertrag_Mueller_Entwurf.docx");
      await sleep(300);
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
      await click("button", "Start again from the sample data");
      await sleep(200);
      await click(".confirm button", "Delete and start again");
      check(
        await waitFor(
          "document.body.innerText.includes('Started again from the sample data')",
        ),
        "reset announced",
      );
      await sleep(SAVE_DELAY);
      await open("/projects", { keep: true });
      check(
        !(await text()).includes("Kaufvertrag Mueller Entwurf"),
        "reset removed the project",
      );
      await open("/terminology/tb-weber", { keep: true });
      check(
        !(await body()).includes("Persistenztest"),
        "reset removed the entry",
      );
      await open("/editor/p-hofmann", { keep: true, wait: 2500 });
      await click("button", "Memories");
      await sleep(400);
      const after = await evaluate(
        "document.querySelector('dialog[open]')?.innerText ?? ''",
      );
      check((await projectUnits()) === 7, "reset emptied the project memory");
    },
  );
