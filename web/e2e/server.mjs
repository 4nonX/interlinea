// The web app on a real team server: signing in, making a project on the
// server from the New project flow, giving work to a translator, the
// translator seeing only their part, translations saved on the server,
// others' changes and presence arriving live, and reporting work done.
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { run, call, ADMIN_PASSWORD } from "./harness.mjs";

export default () =>
  run(
    "Team server",
    async ({ base, send, evaluate, click, text, body, check, sleep, open, setValue, key, samples, fixtures, waitFor, idp }) => {
      const api = (method, path, token, b) => call(base, method, path, token, b);
      const admin = (await api("POST", "/api/auth/password", null, { username: "anna", password: ADMIN_PASSWORD })).token;

      // Signing in from Settings: the server serving the app is offered.
      await open("/settings");
      check(
        await waitFor("document.querySelector(\"input[type=url]\")?.value === location.origin"),
        "the server serving the app is offered",
      );
      check(await waitFor("!!document.querySelector('input[autocomplete=username]')"), "the sign-in form shows");
      await setValue("input[autocomplete=username]", "anna");
      await setValue("input[autocomplete=current-password]", "wrong password here");
      await click("button", "Sign in");
      check(
        await waitFor("document.querySelector('.error[role=alert]')?.textContent.includes('wrong')"),
        "a wrong password is said so",
      );
      await setValue("input[autocomplete=current-password]", ADMIN_PASSWORD);
      await click("button", "Sign in");
      check(await waitFor("document.querySelector('main').innerText.includes('Signed in as anna')"), "signed in");
      {
        const kept = JSON.parse((await evaluate("localStorage.getItem('interlinea.server')")) ?? "null");
        check(kept?.cookie === true && !kept.token, "the browser keeps no token: the server's cookie signs in");
        check(!(await evaluate("document.cookie")).includes("interlinea_session"), "and the page's scripts can't read the cookie");
      }
      check(await waitFor("document.querySelector('main').innerText.includes('Connected')"), "the live feed is connected");

      // Accounts: Anna makes Ben's account and gives and takes a role.
      check(await waitFor("[...document.querySelectorAll('.rail a')].some(a => a.textContent.includes('Accounts'))"), "administrators get Accounts");
      await open("/accounts", { keep: true });
      check(await waitFor("!!document.querySelector('form[aria-label=\"New account\"]')"), "the accounts page opens");
      const field = (i, v) =>
        evaluate(
          `(() => { const el = document.querySelectorAll('form[aria-label="New account"] input')[${i}]; Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, ${JSON.stringify(v)}); el.dispatchEvent(new Event('input', { bubbles: true })); return true; })()`,
        );
      await field(0, "ben");
      await field(1, "Ben Roth");
      await field(2, "ben@agentur.example");
      await field(3, "short");
      await sleep(100);
      await click("button", "Make the account");
      check(await waitFor("document.querySelector('form[aria-label=\"New account\"] .error')?.textContent.includes('12')"), "a short password is refused at its field");
      await field(3, "ben's long password");
      await sleep(100);
      await click("button", "Make the account");
      check(await waitFor("document.querySelector('#acc-detail')?.textContent === 'Ben Roth'"), "the new account opens");
      const ben = (await api("GET", "/api/users", admin)).find((u) => u.username === "ben");
      check(!!ben && ben.email === "ben@agentur.example" && ben.hasPassword, "the account is on the server");
      const pickIn = (form, i, v) =>
        evaluate(
          `(() => { const s = document.querySelectorAll('form[aria-label="${form}"] select')[${i}]; Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s, ${JSON.stringify(v)}); s.dispatchEvent(new Event('change', { bubbles: true })); return true; })()`,
        );
      await pickIn("Give a role", 0, "memory");
      await sleep(100);
      await pickIn("Give a role", 2, "read");
      await sleep(100);
      await click("form[aria-label='Give a role'] button", "Give the role");
      check(await waitFor("document.querySelector('.account-detail').innerText.includes('All memories')"), "the role is listed");
      let gs = await api("GET", `/api/users/${ben.id}/grants`, admin);
      check(gs.length === 1 && gs[0].kind === "memory" && gs[0].resource === "*" && gs[0].role === "read", "the role is on the server");
      await click(".account-detail table button", "Remove");
      check(await waitFor("document.querySelector('.account-detail').innerText.includes('No roles')"), "the role is taken away");
      // Roles of one's own: a reviewer who suggests and approves but does
      // not edit, given, changed, renamed and deleted.
      check(
        await waitFor("document.querySelectorAll('.role-table tbody tr').length === 5 && document.querySelector('.role-table').innerText.includes('Comes with the server')"),
        "the five roles that come with the server are listed",
      );
      check(
        await evaluate("[...document.querySelectorAll('.role-table tbody tr')][2].querySelectorAll('input[type=checkbox]:checked').length === 3 && [...document.querySelectorAll('.role-table tbody input[type=checkbox]')].every(i => i.disabled)"),
        "they show what they allow and cannot be changed",
      );
      await setValue("form[aria-label='New role'] input:not([type=checkbox])", "Reviewer");
      await evaluate("[...document.querySelectorAll('form[aria-label=\"New role\"] label.check')].filter(l => /Suggest|Approve/.test(l.textContent)).forEach(l => l.querySelector('input').click())");
      await sleep(100);
      await click("form[aria-label='New role'] button", "Make the role");
      check(await waitFor("document.querySelectorAll('.role-table tbody tr').length === 6"), "the new role is listed");
      let roles = await api("GET", "/api/roles", admin);
      check(roles.length === 6 && roles[5].name === "Reviewer" && roles[5].permissions.join("+") === "read+suggest+approve", "the role is on the server");
      // It is offered where roles are given.
      await pickIn("Give a role", 0, "termbase");
      await sleep(100);
      check(await evaluate("[...document.querySelectorAll('form[aria-label=\"Give a role\"] select')[2].options].some(o => o.value === 'Reviewer')"), "the role can be given");
      await pickIn("Give a role", 2, "Reviewer");
      await sleep(100);
      await click("form[aria-label='Give a role'] button", "Give the role");
      check(await waitFor("document.querySelector('.account-detail').innerText.includes('All termbases')"), "the reviewer's role is listed");
      gs = await api("GET", `/api/users/${ben.id}/grants`, admin);
      check(gs.length === 1 && gs[0].role === "Reviewer", "the grant names the role");
      check(
        await waitFor("[...document.querySelectorAll('.role-table tbody tr')][5].innerText.includes('1') && [...document.querySelectorAll('.role-table tbody tr')][5].querySelector('.btn')?.disabled"),
        "a role someone has cannot be deleted",
      );
      // Edit is added to it: the change is on the server.
      await evaluate("[...document.querySelectorAll('.role-table tbody tr')][5].querySelectorAll('input[type=checkbox]')[2].click()");
      check(await waitFor("document.querySelector('.toast')?.textContent.includes('Reviewer now allows: edit')"), "changing the role is said");
      roles = await api("GET", "/api/roles", admin);
      check(roles[5].permissions.join("+") === "read+suggest+edit+approve", "the role's permissions changed on the server");
      await click(".account-detail table button", "Remove");
      check(await waitFor("document.querySelector('.account-detail').innerText.includes('No roles')"), "the reviewer's role is taken away");
      check(await waitFor("![...document.querySelectorAll('.role-table tbody tr')][5].querySelector('.btn')?.disabled"), "now it can be deleted");
      await click(".role-table tbody tr:nth-child(6) button", "Delete");
      await sleep(150);
      await click(".role-table .confirm button", "Delete");
      check(await waitFor("document.querySelectorAll('.role-table tbody tr').length === 5"), "the role is deleted");
      check((await api("GET", "/api/roles", admin)).length === 5, "and gone from the server");

      await evaluate("[...document.querySelectorAll('.account-switches label')].find(l => l.textContent.includes('Disabled')).querySelector('input').click()");
      check(await waitFor("document.querySelector('.account-list').innerText.includes('disabled')"), "disabling shows in the list");
      check((await api("GET", `/api/users/${ben.id}`, admin)).disabled, "the account is disabled on the server");
      await evaluate("[...document.querySelectorAll('.account-switches label')].find(l => l.textContent.includes('Disabled')).querySelector('input').click()");
      check(await waitFor("!document.querySelector('.account-list').innerText.includes('disabled')"), "enabled again");

      // A new project, kept on the server.
      await open("/new", { keep: true, wait: 0 });
      await sleep(600);
      await click("button", "Continue to files and languages");
      await sleep(300);
      const doc = await send("DOM.getDocument", { depth: -1, pierce: true });
      const q = await send("DOM.querySelector", {
        nodeId: doc.root.nodeId,
        selector: "input[type=file][multiple]:not([webkitdirectory])",
      });
      await send("DOM.setFileInputFiles", { nodeId: q.nodeId, files: [`${samples}/Mietvertrag_Hofmann_2026.docx`] });
      check(await waitFor("document.querySelectorAll('.file-table tbody tr').length === 1"), "the file is listed");
      await setValue("#pname", "Hofmann Team");
      await sleep(300);
      await click("button", "Continue to memories");
      await sleep(400);
      await click("button", "Continue to terminology");
      await sleep(400);
      // Layered over the client's master, which goes to the server with it.
      await evaluate("[...document.querySelectorAll('main label')].find(l => l.textContent.includes('Kanzlei Weber') && l.querySelector('input[type=checkbox]')).querySelector('input').click()");
      await sleep(200);
      await evaluate("[...document.querySelectorAll('main label')].find(l => l.textContent.includes('Project termbase, then merge into the masters')).querySelector('input').click()");
      await sleep(200);
      check(
        await evaluate("[...document.querySelectorAll('main label')].find(l => l.textContent.includes('Project termbase, then merge into the masters')).querySelector('input').checked && [...document.querySelectorAll('main label')].find(l => l.textContent.includes('Kanzlei Weber') && l.querySelector('input[type=checkbox]')).querySelector('input').checked"),
        "the project is layered over the master",
      );
      await click("button", "Continue to review");
      check(await waitFor("!!document.querySelector('.analysis')", 15000), "review analyses the file");
      check((await text()).includes("Where the project is kept"), "signed in, the review asks where the project is kept");
      await evaluate("document.querySelector('input[name=where]:not(:checked)').click()");
      await sleep(200);
      await click("button", "Create on the server");
      check(
        await waitFor("document.querySelector('main').innerText.includes('Give out work')", 20000),
        "the project is made on the server and its page opens",
      );
      const t = await body();
      check(/Created project “Hofmann Team” on 127\.0\.0\.1/.test(t), "creation says where: " + (t.match(/Created project[^\n]*/)?.[0] ?? ""));
      const projects = await api("GET", "/api/projects", admin);
      const p = projects.find((x) => x.settings.name === "Hofmann Team");
      check(!!p && p.settings.terminology.location === "server" && !!p.settings.terminology.termbase, "the server has the project with its termbase");
      const file = p.settings.files[0].name;
      const lang = p.settings.targetLangs[0];
      const segs = await api("GET", `/api/projects/${p.id}/segments?file=${encodeURIComponent(file)}&lang=${lang}`, admin);
      check(segs.length > 4, `the server has the segments (${segs.length})`);
      const memories = await api("GET", "/api/memories", admin);
      check(
        memories.some((m) => m.name === `Hofmann Team (${lang})`),
        "the project's own memory is on the server: " + memories.map((m) => m.name).join(", "),
      );

      check((await text()).includes("People with access"), "managers decide who may use the project");

      // The manager changes the project on the server: a file in and out,
      // and a check turned off.
      check(await waitFor("document.querySelector('main').innerText.includes('Languages and memories')", 10000), "managers get the project's settings");
      const dir = mkdtempSync(join(tmpdir(), "il-e2e-"));
      writeFileSync(join(dir, "Hinweise.txt"), "Der Schlüssel liegt beim Hausmeister.\n\nDie Heizung wird im Oktober eingeschaltet.\n");
      const doc2 = await send("DOM.getDocument", { depth: -1, pierce: true });
      const fin = await send("DOM.querySelector", { nodeId: doc2.root.nodeId, selector: "input[aria-label='Files to add']" });
      await send("DOM.setFileInputFiles", { nodeId: fin.nodeId, files: [join(dir, "Hinweise.txt")] });
      check(await waitFor("[...document.querySelectorAll('.server-file')].some(li => li.textContent.includes('Hinweise.txt'))", 15000), "the added file is listed");
      let sp = await api("GET", `/api/projects/${p.id}`, admin);
      check(sp.settings.files.some((f) => f.name === "Hinweise.txt"), "the file is on the server");
      const hs = await api("GET", `/api/projects/${p.id}/segments?file=Hinweise.txt&lang=${lang}`, admin);
      check(hs.length === 2, `the file's segments are on the server (${hs.length})`);
      await evaluate("[...document.querySelectorAll('.server-file')].find(li => li.textContent.includes('Hinweise.txt')).querySelector('button').click()");
      await sleep(150);
      await evaluate("[...document.querySelectorAll('.server-file')].find(li => li.textContent.includes('Hinweise.txt')).querySelector('.confirm button').click()");
      check(await waitFor("![...document.querySelectorAll('.server-file')].some(li => li.textContent.includes('Hinweise.txt'))", 10000), "the file is removed");
      sp = await api("GET", `/api/projects/${p.id}`, admin);
      check(!sp.settings.files.some((f) => f.name === "Hinweise.txt"), "and gone on the server");
      const before = (sp.settings.qa.disabled ?? []).length;
      await evaluate("document.querySelector('section[aria-labelledby=sps-checks] input[type=checkbox]:checked').click()");
      await sleep(150);
      await click("button", "Save settings");
      let qa = null;
      for (let i = 0; i < 40 && !qa; i++) {
        await sleep(200);
        const now = await api("GET", `/api/projects/${p.id}`, admin);
        if ((now.settings.qa.disabled ?? []).length === before + 1) qa = now.settings.qa;
      }
      check(!!qa, "the settings are saved on the server");

      // Work for Ben: the first two segments.
      check(await waitFor("[...document.querySelectorAll('form[aria-label=\"Give out work\"] select')[0].options].some(o => o.textContent.includes('Ben Roth'))"), "Ben can be chosen");
      const pick = (i, v) =>
        evaluate(
          `(() => { const s = document.querySelectorAll('form[aria-label="Give out work"] select')[${i}]; Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s, ${JSON.stringify(v)}); s.dispatchEvent(new Event('change', { bubbles: true })); return true; })()`,
        );
      await pick(0, String(ben.id));
      await sleep(100);
      await pick(2, file);
      await sleep(100);
      await pick(3, lang);
      await sleep(300);
      await pick(4, segs[0].id);
      await sleep(100);
      await pick(5, segs[1].id);
      await sleep(100);
      await click("button", "Give out");
      check(await waitFor("document.querySelector('main').innerText.includes('Ben Roth')"), "the work is listed");
      const benTok = (await api("POST", "/api/auth/password", null, { username: "ben", password: "ben's long password" })).token;
      const mine = await api("GET", "/api/me/assignments", benTok);
      check(mine.length === 1 && mine[0].fromSegment === segs[0].id && mine[0].toSegment === segs[1].id, "Ben has the two segments");

      // Ben signs in here.
      await open("/settings", { keep: true });
      await click("button", "Sign out");
      check(await waitFor("!!document.querySelector('input[autocomplete=username]')"), "signed out");
      await setValue("input[autocomplete=username]", "ben");
      await setValue("input[autocomplete=current-password]", "ben's long password");
      await click("button", "Sign in");
      check(await waitFor("document.querySelector('main').innerText.includes('Signed in as Ben Roth')"), "Ben is signed in");
      await open("/projects", { keep: true });
      check(
        await waitFor("document.querySelector('main').innerText.includes('On the team server') && document.querySelector('main').innerText.includes('Translating')"),
        "Ben sees the project and his work",
      );
      await click("a", "Hofmann Team");
      check(await waitFor("!!document.querySelector('.seg.is-active .target-editor')", 20000), "the project opens in the editor");
      check(
        (await evaluate("document.querySelectorAll('.seg.is-locked').length")) === segs.length - 2,
        "only Ben's segments can be changed",
      );
      check(
        (await evaluate("document.querySelector('.seg.is-locked .seg-status')?.title ?? ''")).includes("Not assigned to you"),
        "the others say why",
      );

      // Ben translates and confirms his first segment: it is on the server.
      await evaluate(
        "document.querySelector('.target-editor').textContent = ''; document.querySelector('.target-editor').dispatchEvent(new Event('input', { bubbles: true }))",
      );
      await sleep(150);
      await evaluate("document.querySelector('.target-editor').focus()");
      await send("Input.insertText", { text: "Tenancy agreement" });
      await sleep(200);
      await key("Enter", "Enter", 13, 2);
      let saved = null;
      for (let i = 0; i < 50 && !saved; i++) {
        await sleep(200);
        const now = await api("GET", `/api/projects/${p.id}/segments?file=${encodeURIComponent(file)}&lang=${lang}`, admin);
        const s1 = now.find((x) => x.id === segs[0].id);
        if (s1.status === "confirmed") saved = s1;
      }
      check(
        !!saved && saved.changedBy === "ben" && JSON.stringify(saved.target).includes("Tenancy agreement"),
        "the confirmed translation is saved on the server: " + JSON.stringify(saved?.target),
      );

      // Ben types in his second segment while Anna saves it first: Ben is
      // told, and sees Anna's translation.
      check(await waitFor("!!document.querySelector('.seg.is-active .target-editor')"), "the next segment is open");
      await evaluate("document.querySelector('.seg.is-active .target-editor').focus()");
      await send("Input.insertText", { text: " Ben's words" });
      const s2 = (await api("GET", `/api/projects/${p.id}/segments?file=${encodeURIComponent(file)}&lang=${lang}`, admin)).find((x) => x.id === segs[1].id);
      await api(
        "PUT",
        `/api/projects/${p.id}/segments?file=${encodeURIComponent(file)}&lang=${lang}&segment=${s2.id}&version=${s2.version}`,
        admin,
        { target: [{ text: "Anna was first" }], status: "draft" },
      );
      check(await waitFor("document.body.innerText.includes('changed this segment on the server first')", 10000), "Ben is told someone saved first");
      check(await waitFor("document.querySelector('.segments').innerText.includes('Anna was first')", 10000), "Ben sees the translation saved first");
      const kept = (await api("GET", `/api/projects/${p.id}/segments?file=${encodeURIComponent(file)}&lang=${lang}`, admin)).find((x) => x.id === segs[1].id);
      check(JSON.stringify(kept.target).includes("Anna was first") && kept.changedBy === "anna", "the server keeps the translation saved first");

      // Comments on a segment are kept on the server for everyone: Ben's is
      // saved there, Anna's arrives while he works, and neither touches the translation.
      {
        const box = "section[aria-label=Comments]";
        const segOnServer = async () => {
          const all = await api("GET", `/api/projects/${p.id}/segments?file=${encodeURIComponent(file)}&lang=${lang}`, admin);
          return all.find((x) => (x.comments ?? []).some((c) => c.text === "Tenancy or lease?")) ?? null;
        };
        await setValue(`${box} textarea`, "Tenancy or lease?");
        await sleep(100);
        await click(`${box} button`, "Add comment");
        check(await waitFor(`document.querySelector(${JSON.stringify(box + " .comment-list")})?.innerText.includes('Tenancy or lease?')`, 10000), "Ben's comment is shown");
        const commented = await segOnServer();
        check(!!commented && commented.comments[0].by === "ben", "and is kept on the server under his name");
        const versionBefore = commented.version;
        await api("POST", `/api/projects/${p.id}/comments?file=${encodeURIComponent(file)}&lang=${lang}&segment=${commented.id}`, admin, { text: "Lease, as in the client's glossary." });
        check(await waitFor(`document.querySelectorAll(${JSON.stringify(box + " .comment-list li")}).length === 2`, 10000), "Anna's comment arrives while Ben works");
        check((await evaluate(`document.querySelector(${JSON.stringify(box + " .comment-list")}).innerText`)).includes("Lease, as in the client's glossary."), "with her words");
        check((await segOnServer()).version === versionBefore, "comments leave the segment's version alone");
        // Ben deletes his own; Anna's has no button for him.
        check((await evaluate(`document.querySelectorAll(${JSON.stringify(box + " .comment-list li button")}).length`)) === 1, "Ben may delete only his own comment");
        await evaluate(`document.querySelector(${JSON.stringify(box + " .comment-list li button")}).click()`);
        check(await waitFor(`document.querySelectorAll(${JSON.stringify(box + " .comment-list li")}).length === 1`, 10000), "his comment is deleted");
        const r = await fetch(`${base}/api/projects/${p.id}/comments?file=${encodeURIComponent(file)}&lang=${lang}&segment=${commented.id}&comment=nope`, { method: "DELETE", headers: { Authorization: `Bearer ${admin}` } });
        check(r.status === 404, "a comment that is not there is not found");
      }

      // The memory search asks the server: Ben's confirmed translation was
      // not in what the browser loaded, but is found.
      await key("k", "KeyK", 75, 2);
      await evaluate(
        "(() => { const el = document.getElementById('lookup-q'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, 'Tenancy'); el.dispatchEvent(new Event('input', { bubbles: true })); })()",
      );
      check(
        await waitFor("[...document.querySelectorAll('.lookup-list li')].some(li => li.textContent.includes('Tenancy agreement'))", 10000),
        "the memory search runs on the server",
      );
      await key("Escape", "Escape", 27);

      // Anna translates segment 4 elsewhere: it shows here without reloading.
      const s4 = segs[3];
      await api(
        "PUT",
        `/api/projects/${p.id}/segments?file=${encodeURIComponent(file)}&lang=${lang}&segment=${s4.id}&version=${s4.version}`,
        admin,
        { target: [{ text: "Translated by Anna" }], status: "draft" },
      );
      check(await waitFor("document.querySelector('.segments').innerText.includes('Translated by Anna')", 10000), "others' translations arrive live");

      // Anna opens segment 5: Ben sees her there.
      await api("PUT", `/api/projects/${p.id}/presence`, admin, { file, lang, segment: segs[4].id });
      check(
        await waitFor("[...document.querySelectorAll('.seg-presence')].some(x => x.getAttribute('aria-label').includes('anna'))", 10000),
        "others' open segments are marked",
      );
      const benThere = await api("GET", `/api/projects/${p.id}/presence`, admin);
      check(benThere.some((x) => x.userName === "Ben Roth"), "the server knows where Ben is");

      // Ben reports his work done on the project's page.
      await open(`/project/srv-${p.id}`, { keep: true });
      check(await waitFor("!!document.querySelector('input[type=checkbox][aria-label*=\"work is done\"]')", 15000), "Ben sees his work");
      check(!(await text()).includes("Give out work"), "only managers give out work");
      await evaluate("document.querySelector('input[type=checkbox][aria-label*=\"work is done\"]').click()");
      let done = false;
      for (let i = 0; i < 30 && !done; i++) {
        await sleep(200);
        done = (await api("GET", "/api/me/assignments", benTok))[0].done;
      }
      check(done, "the work is reported done on the server");

      // Ben adds a term to the project termbase on the server, from Terminology.
      const ptb = p.settings.terminology.termbase;
      await open("/terminology", { keep: true });
      check(await waitFor(`[...document.querySelectorAll('.tb-link')].some(a => a.textContent.includes(${JSON.stringify(ptb)}))`, 10000), "server termbases are listed");
      await click(".tb-link", ptb);
      check(await waitFor(`document.querySelector('#entries-title')?.textContent === ${JSON.stringify(ptb)}`, 15000), "the server termbase opens");
      await click(".new-entry-bar button", "New entry");
      await sleep(200);
      await setValue("#ne-text", "Hausmeisterschlüssel");
      await click(".new-entry button", "Add entry");
      let added = null;
      for (let i = 0; i < 40 && !added; i++) {
        await sleep(200);
        const page = await api("GET", `/api/termbases/${encodeURIComponent(ptb)}/entries?text=Hausmeisterschlüssel`, admin);
        added = page.entries[0] ?? null;
      }
      check(!!added && added.createdBy === "ben", "the new entry is on the server");

      // Anna submits the project's terms to the master and approves them, and
      // finds Ben's confirmed translation in the project memory.
      await open("/settings", { keep: true });
      await click("button", "Sign out");
      check(await waitFor("!!document.querySelector('input[autocomplete=username]')"), "Ben signed out");
      await setValue("input[autocomplete=username]", "anna");
      await setValue("input[autocomplete=current-password]", ADMIN_PASSWORD);
      await click("button", "Sign in");
      check(await waitFor("document.querySelector('main').innerText.includes('Signed in as anna')"), "Anna signed in again");
      await open(`/terminology/srv:${ptb}`, { keep: true });
      check(await waitFor("!!document.querySelector('form[aria-label=\"Submit for review\"]')", 15000), "a project termbase can be submitted for review");
      await click("form[aria-label='Submit for review'] button", "Submit to");
      check(await waitFor("location.hash.startsWith('#/review/')", 10000), "the review opens");
      check(await waitFor("document.querySelector('main').innerText.includes('Waiting (1)')", 10000), "Ben's term waits");
      check(await waitFor("document.querySelector('main').innerText.includes('Approve all waiting')", 10000), "Anna opens the review");
      const master = p.settings.terminology.masters[0];
      const lookUp = () => api("GET", `/api/termbases/${encodeURIComponent(master)}/lookup?text=Hausmeisterschlüssel&lang=${encodeURIComponent(p.settings.sourceLang)}`, admin);
      const choosePart = (label, value) =>
        evaluate(
          `(() => { const el = [...document.querySelectorAll('form[aria-label="Give someone a part"] label')].find(l => l.textContent.startsWith(${JSON.stringify(label)})).querySelector('select'); Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(el, ${JSON.stringify(value)}); el.dispatchEvent(new Event('change', { bubbles: true })); return true; })()`,
        );

      // Reviewers: nobody has a part yet. Ben can't be given one while he
      // may not approve changes to the master; once he may, he can.
      check(await waitFor("document.querySelector('main').innerText.includes('Nobody is given a part')", 10000), "nobody has a part of the request yet");
      await choosePart("Who", "ben");
      await click("form[aria-label='Give someone a part'] button", "Give the part");
      check(await waitFor("document.querySelector('.toast')?.textContent.includes('may not approve changes')", 10000), "someone who may not approve is not made a reviewer");
      await api("PUT", "/api/grants", admin, { userId: ben.id, kind: "termbase", resource: master, role: "approve" });
      await choosePart("Who", "ben");
      await choosePart("Part", "kind:new");
      await click("form[aria-label='Give someone a part'] button", "Give the part");
      check(await waitFor("document.querySelector('.mr-reviewers')?.innerText.includes('new entries: 1 of 1 to decide')", 10000), "Ben reviews the new entries, and how far he is shows");
      check(await waitFor("document.querySelector('.mr-item').innerText.includes('for ben to review')"), "the entry says who reviews it");
      await click(".mr-reviewers button", "Remove");
      check(await waitFor("document.querySelector('main').innerText.includes('Nobody is given a part')", 10000), "the part is taken away again");

      // Postponed with a note for the author, the entry is decided later.
      await setValue(".mr-note", "Bitte mit Quelle");
      await click(".mr-item button", "Postpone");
      check(await waitFor("document.querySelector('main').innerText.includes('Nothing is waiting.')", 10000), "the entry is postponed");
      await click("[role=tab]", "Decided");
      check(await waitFor("document.querySelector('.mr-item')?.innerText.includes('“Bitte mit Quelle”, anna')", 10000), "the note is with the postponed entry");
      check(
        await evaluate("![...document.querySelectorAll('.mr-item button')].some(b => b.textContent.includes('Postpone')) && [...document.querySelectorAll('.mr-item button')].some(b => b.textContent.includes('Approve'))"),
        "what was postponed is approved or rejected, not put off again",
      );
      await click(".mr-item button", "Approve");
      let inMaster = [];
      for (let i = 0; i < 40 && inMaster.length === 0; i++) {
        await sleep(200);
        inMaster = await lookUp();
      }
      check(inMaster.length === 1, `the approved term is in the master ${master}`);

      // The merge is undone: the term leaves the master, and the request says so.
      check(await waitFor("[...document.querySelectorAll('.mr-item button')].some(b => b.textContent.includes('Undo the merge'))", 10000), "a merged entry can be taken out again");
      await click(".mr-item button", "Undo the merge");
      await sleep(150);
      check((await text()).includes(`Take this out of ${master} again?`), "undoing asks first");
      await click(".mr-item .confirm button", "Undo");
      check(await waitFor(`document.querySelector('.toast')?.textContent.includes(${JSON.stringify(`Took the merge back out of ${master}`)})`, 10000), "undoing is said");
      check(await waitFor("document.querySelector('.mr-item')?.innerText.includes('It is submitted again once the project has changed it.')", 10000), "the entry says what becomes of it");
      check((await lookUp()).length === 0, "the term is out of the master again");
      await click("[role=tab]", "Waiting");
      await sleep(300);
      await click("button", "Close the request");
      await sleep(150);
      await click(".confirm button", "Close");
      check(await waitFor("document.querySelector('main').innerText.includes('Closed on')", 10000), "the request is closed");

      await open("/memories", { keep: true });
      check(await waitFor(`document.querySelector('main').innerText.includes(${JSON.stringify(`Hofmann Team (${lang})`)})`, 10000), "server memories are listed");
      await evaluate(`[...document.querySelectorAll('section[aria-labelledby=mem-server] tr')].find(tr => tr.textContent.includes(${JSON.stringify(`Hofmann Team (${lang})`)})).querySelector('button').click()`);
      check(await waitFor("!!document.querySelector('#srvmem-title')"), "a server memory opens");
      await setValue("section[aria-labelledby=srvmem-title] input[type=search]", "Tenancy");
      await evaluate("document.querySelector('section[aria-labelledby=srvmem-title] input[type=checkbox]').click()");
      await sleep(100);
      await click("section[aria-labelledby=srvmem-title] button", "Search");
      check(await waitFor("document.querySelector('section[aria-labelledby=srvmem-title]').innerText.includes('Tenancy agreement')", 10000), "the memory is searched on the server");

      // The units of a memory on the server: a unit is added, found, its translation
      // changed, and the unit deleted.
      {
        const panel = "section[aria-labelledby=srvmem-title]";
        await click(`${panel} button`, "Add a unit");
        check(await waitFor("!!document.querySelector('dialog[open] textarea')"), "server memory: the dialog for a new unit opens");
        check(await evaluate("[...document.querySelectorAll('dialog[open] button')].find(b => b.textContent.includes('Add the unit')).disabled"), "server memory: a unit needs its sentence and translation");
        await evaluate(
          `(() => { const set = (el, v) => { Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true })); }; const ts = document.querySelectorAll('dialog[open] textarea'); set(ts[0], "Die Hausordnung gilt."); set(ts[1], "The house rules apply."); return true; })()`,
        );
        await sleep(100);
        await click("dialog[open] button", "Add the unit");
        check(await waitFor("document.querySelector('.toast')?.textContent.includes('Added the unit to')", 10000), "server memory: the unit is added");
        await setValue(`${panel} input[type=search]`, "Hausordnung");
        if (await evaluate(`document.querySelector(${JSON.stringify(panel)} + ' input[type=checkbox]').checked`))
          await evaluate(`document.querySelector(${JSON.stringify(panel)} + ' input[type=checkbox]').click()`);
        await sleep(100);
        await click(`${panel} button`, "Search");
        check(await waitFor(`document.querySelector(${JSON.stringify(panel)} + ' table[aria-label="Units found"]')?.innerText.includes("The house rules apply.")`, 10000), "server memory: the new unit is found");
        await click(`${panel} table button`, "Change");
        check(await waitFor("!!document.querySelector('dialog[open] .unit-target .target-editor')"), "server memory: the unit opens to be changed");
        check((await evaluate("document.querySelector('dialog[open] .unit-source').innerText")).includes("Die Hausordnung gilt."), "server memory: its sentence is shown to read");
        await evaluate("(() => { const el = document.querySelector('dialog[open] .unit-target .target-editor'); el.focus(); const r = document.createRange(); r.selectNodeContents(el); r.collapse(false); const s = getSelection(); s.removeAllRanges(); s.addRange(r); return document.execCommand('insertText', false, ' Geändert.'); })()");
        await sleep(100);
        await click("dialog[open] button", "Save");
        check(await waitFor("document.querySelector('.toast')?.textContent.includes('Saved the unit')", 10000), "server memory: the changed translation is saved");
        check(await waitFor(`document.querySelector(${JSON.stringify(panel)} + ' table[aria-label="Units found"]')?.innerText.includes('Geändert.')`, 10000), "server memory: the search shows the changed translation");
        await evaluate(`document.querySelector(${JSON.stringify(panel)} + ' table .cell-action .row > button:last-child').click()`);
        await sleep(150);
        check((await text()).includes("Delete this unit?"), "server memory: deleting a unit asks first");
        await click(`${panel} table .confirm button`, "Delete");
        check(await waitFor("document.querySelector('.toast')?.textContent.includes('Deleted the unit')", 10000), "server memory: the unit is deleted");
        check(await waitFor(`document.querySelector(${JSON.stringify(panel)}).innerText.includes('Nothing found.')`, 10000), "server memory: and no longer found");
        // A document and its translation are aligned into the memory on the server.
        await click(`${panel} button`, "Align documents");
        await waitFor("document.querySelectorAll('dialog[open] input[type=file]').length === 2");
        for (const [n, f] of [[1, "Mietvertrag_de.txt"], [2, "Mietvertrag_fr.txt"]]) {
          const doc = await send("DOM.getDocument", { depth: -1, pierce: true });
          const q = await send("DOM.querySelector", { nodeId: doc.root.nodeId, selector: `dialog[open] label:nth-of-type(${n}) input[type=file]` });
          await send("DOM.setFileInputFiles", { nodeId: q.nodeId, files: [`${fixtures}/align/${f}`] });
        }
        await sleep(300);
        await click("dialog[open] button", "Pair the sentences");
        check(await waitFor("document.querySelectorAll('dialog[open] svg [data-line]').length === 5", 20000), "server memory: two documents are aligned");
        await click("dialog[open] button", "Add 4 units");
        check(await waitFor("document.querySelector('.toast')?.textContent.includes('Added 4 units to')", 10000), "server memory: the aligned units are added");
        await setValue(`${panel} input[type=search]`, "Werktag");
        await sleep(100);
        await click(`${panel} button`, "Search");
        check(await waitFor(`document.querySelector(${JSON.stringify(panel)} + ' table[aria-label="Units found"]')?.innerText.includes("troisième jour ouvrable")`, 10000), "server memory: and found on the server");
        // From the page, documents are aligned into a memory made on the server for them.
        await click("section[aria-labelledby=mem-server] > .row > button", "Align documents");
        check(await waitFor("document.querySelector('dialog[open] .dialog-title')?.textContent === 'Align documents'"), "server memory: alignment opens from the page");
        await evaluate("(() => { const s = document.querySelectorAll('dialog[open] select')[0]; s.selectedIndex = s.options.length - 1; s.dispatchEvent(new Event('change', { bubbles: true })); return true; })()");
        await waitFor("document.querySelectorAll('dialog[open] select').length === 3");
        await setValue("dialog[open] .field-grid input", "E2E Altbestand");
        await evaluate("(() => { const s = document.querySelectorAll('dialog[open] select')[2]; Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s, 'fr-FR'); s.dispatchEvent(new Event('change', { bubbles: true })); return true; })()");
        for (const [n, f] of [[2, "Mietvertrag_de.txt"], [3, "Mietvertrag_fr.txt"]]) {
          const doc = await send("DOM.getDocument", { depth: -1, pierce: true });
          const q = await send("DOM.querySelector", { nodeId: doc.root.nodeId, selector: `dialog[open] label:nth-of-type(${n}) input[type=file]` });
          await send("DOM.setFileInputFiles", { nodeId: q.nodeId, files: [`${fixtures}/align/${f}`] });
        }
        await sleep(300);
        await click("dialog[open] button", "Pair the sentences");
        check(await waitFor("document.querySelectorAll('dialog[open] svg [data-line]').length === 5", 20000), "server memory: the documents are paired for a new memory");
        await click("dialog[open] button", "Add 4 units to E2E Altbestand");
        check(await waitFor("document.querySelector('.toast')?.textContent.includes('Added 4 units to E2E Altbestand on')", 10000), "server memory: the memory is made on the server and filled");
        check(await waitFor("[...document.querySelectorAll('section[aria-labelledby=mem-server] tbody tr')].some(tr => tr.textContent.includes('E2E Altbestand') && tr.querySelector('td.num').textContent.trim() === '4')", 10000), "server memory: and listed with its units");
      }

      // Termbases on the server, through their whole life: an earlier
      // version restored, a deleted entry brought back, the master's
      // entries under a project termbase, settings, and a project termbase
      // made and deleted by hand.
      {
        const tbPath = (n) => `/api/termbases/${encodeURIComponent(n)}`;
        const toast = (part) => waitFor(`document.querySelector('.toast')?.textContent.includes(${JSON.stringify(part)})`, 10000);
        const entryOf = (de, en) => ({ languages: [{ lang: "de-DE", terms: [{ text: de, status: "approved" }] }, { lang: "en-GB", terms: [{ text: en, status: "approved" }] }] });
        await api("POST", "/api/termbases", admin, { definition: { name: "E2E Recht", languages: ["de-DE", "en-GB"], fields: [{ name: "Note", level: "entry", kind: "text" }] } });
        const first = await api("POST", `${tbPath("E2E Recht")}/entries`, admin, { entry: entryOf("Nebenkosten", "service charges") });
        await api("PUT", `${tbPath("E2E Recht")}/entries/${first.id}?version=1`, admin, { entry: { ...entryOf("Nebenkosten", "ancillary costs"), id: first.id } });
        await api("POST", `${tbPath("E2E Recht")}/entries`, admin, { entry: entryOf("Staffelmiete", "stepped rent") });

        await open(`/terminology/srv:${encodeURIComponent("E2E Recht")}/srv:${encodeURIComponent("E2E Recht")}:${first.id}`, { keep: true });
        check(await waitFor("document.querySelector('#detail-title')?.textContent.includes('Nebenkosten')", 15000), "a master on the server opens with its entry");
        check(await evaluate("[...document.querySelectorAll('.pane-tools button')].some(b => b.textContent.includes('Settings'))"), "who manages a server termbase has its settings");
        await click("#tab-history", "");
        check(await waitFor("document.querySelector('#panel-history')?.innerText.includes('en-GB: service charges')", 10000), "the history says what each version held");
        await click("#panel-history button", "Restore this version");
        await sleep(150);
        check((await text()).includes("Set the entry back to this version?"), "restoring asks first");
        await click("#panel-history .confirm button", "Restore");
        check(await toast("Restored the version of"), "an earlier version is restored");
        {
          const now = await api("GET", `${tbPath("E2E Recht")}/entries/${first.id}`, admin);
          check(now.version === 3 && now.languages.find((l) => l.lang === "en-GB").terms[0].text === "service charges", "the entry is as it was, as a new version");
        }

        // Deleted, listed, and brought back.
        await click("#tab-entry", "");
        await click(".pane-detail button", "Delete entry");
        await sleep(150);
        await click(".pane-detail .confirm button", "Delete");
        check(await toast("Deleted the entry"), "the entry is deleted");
        await click(".pane-entries [role=tab]", "Deleted");
        check(await waitFor("document.querySelector('.deleted-entry')?.innerText.includes('Nebenkosten')", 10000), "the deleted entry is listed");
        await click(".deleted-entry button", "Bring back");
        check(await toast("Brought back “Nebenkosten”"), "it is brought back");
        check(await waitFor("document.querySelector('.pane-entries').innerText.includes('Nothing deleted')", 10000), "and no longer listed as deleted");
        check((await api("GET", `${tbPath("E2E Recht")}/entries/${first.id}`, admin)).version === 1, "it has its ID again");

        // How the master is reviewed, in its settings.
        await click(".pane-tools button", "Settings");
        check(await waitFor("document.querySelector('dialog[open]')?.innerText.includes('Trusted authors')", 10000), "the settings of a master say how it is reviewed");
        check((await evaluate("document.querySelector('dialog[open]').innerText")).includes("Who may use it"), "and who may use it");
        check(await waitFor("[...document.querySelectorAll('dialog[open] [aria-label=\"Fields that need no approval\"] label.check')].some(l => l.textContent.includes('Note'))", 10000), "the master's fields are offered as needing no approval");
        await evaluate("[...document.querySelectorAll('dialog[open] [aria-label=\"Fields that need no approval\"] label.check')].find(l => l.textContent.includes('Note')).querySelector('input').click()");
        check(await toast("Changes to Note only are merged without review"), "a field that needs no approval is set");
        check(((await api("GET", `${tbPath("E2E Recht")}/review-rules`, admin)).minorFields ?? []).join() === "Note", "the rule is on the server");
        await click("dialog[open] button", "Done");

        // A project termbase made by hand over the master.
        await click(".pane-tbs button", "New project termbase");
        check(await waitFor("!!document.querySelector('form[aria-label=\"New project termbase\"]')"), "a project termbase can be made by hand");
        await setValue("#new-ptb", "E2E Fall");
        check(await waitFor("[...document.querySelectorAll('form[aria-label=\"New project termbase\"] label.check')].some(l => l.textContent.includes('E2E Recht'))", 10000), "the masters of the server are offered to base it on");
        await evaluate("[...document.querySelectorAll('form[aria-label=\"New project termbase\"] label.check')].find(l => l.textContent.includes('E2E Recht')).querySelector('input').click()");
        await sleep(100);
        await click("form[aria-label='New project termbase'] button", "Create");
        check(await toast("Created project termbase “E2E Fall”"), "the project termbase is made");
        check(await waitFor("[...document.querySelectorAll('.pane-entries [role=tab]')].some(t => t.textContent.includes('Master (2)'))", 15000), "it shows its master's entries in a view of their own");
        check((await text()).includes("Based on E2E Recht."), "and says what it is based on");
        await click(".pane-entries [role=tab]", "Master (2)");
        check(await waitFor("document.querySelector('#master-entry-title')?.textContent.length > 0", 10000), "a master's entry opens to read");
        await click(".pane-detail button", "Leave out of this project");
        await sleep(150);
        await click(".pane-detail .confirm button", "Leave out");
        check(await toast("Left the entry out of the project"), "a master's entry is left out");
        check(await waitFor("document.querySelector('.left-out')?.innerText.includes('Left out of this project')", 10000), "what is left out is listed, by its term");
        check((await api("GET", `${tbPath("E2E Fall")}/removals`, admin)).length === 1, "the server has the removal");
        await click(".left-out button", "Bring back");
        check(await toast("Brought the entry back into the project"), "it is brought back into the project");
        check(await waitFor("!document.querySelector('.left-out')", 10000), "nothing is left out any more");
        await click(".pane-detail button", "Change in this project");
        check(await toast("Took the entry into the project"), "a master's entry is taken into the project to be changed");
        check(await waitFor("!!document.querySelector('#detail-title') && [...document.querySelectorAll('.pane-entries [role=tab]')].some(t => t.textContent.includes('Master (1)'))", 15000), "the project's copy opens, and the master has one entry less to show");
        check((await api("GET", `${tbPath("E2E Fall")}/entries`, admin)).total === 1, "the copy is in the project termbase");

        // The master stays while a project termbase is based on it.
        await open(`/terminology/srv:${encodeURIComponent("E2E Recht")}`, { keep: true });
        check(await waitFor("[...document.querySelectorAll('.pane-tools button')].some(b => b.textContent.includes('Settings'))", 15000), "the master opens again");
        await click(".pane-tools button", "Settings");
        check(await waitFor("document.querySelector('dialog[open]')?.innerText.includes('E2E Fall is based on this termbase.')", 10000), "a master says which project termbase stands in the way of deleting it");
        await click("dialog[open] button", "Done");
        await open(`/terminology/srv:${encodeURIComponent("E2E Fall")}`, { keep: true });
        check(await waitFor("[...document.querySelectorAll('.pane-tools button')].some(b => b.textContent.includes('Settings'))", 15000), "the project termbase opens again");
        await click(".pane-tools button", "Settings");
        check(await waitFor("document.querySelector('dialog[open]')?.innerText.includes('The masters keep theirs.')", 10000), "deleting a project termbase says what it takes");
        await click("dialog[open] .settings-danger button", "Delete termbase");
        await sleep(150);
        await click("dialog[open] .settings-danger .confirm button", "Delete");
        check(await toast("Deleted E2E Fall"), "the project termbase is deleted");
        check((await api("GET", "/api/termbases", admin)).every((t) => t.name !== "E2E Fall"), "it is gone from the server");
      }

      // Eva signs in through the identity provider: the server makes her an
      // account (with no roles) on her first sign-in.
      await open("/settings", { keep: true });
      await click("button", "Sign out");
      check(await waitFor("document.querySelector('main').innerText.includes('Sign in with Test IdP')"), "the provider is offered");
      idp.signInAs({ sub: "eva-1", preferred_username: "eva", name: "Eva Brandt", email: "eva@agentur.example", email_verified: true });
      await click("button", "Sign in with Test IdP");
      check(await waitFor("document.querySelector('main')?.innerText.includes('Signed in as Eva Brandt')", 20000), "signed in through the provider");
      check(!(await evaluate("location.hash")).includes("interlinea-code"), "the one-time code is gone from the address");
      const eva = (await api("GET", "/api/users", admin)).find((u) => u.username === "eva");
      check(!!eva && !eva.hasPassword && eva.email === "eva@agentur.example" && !eva.admin, "her account was made, without a password or roles");
      const ids = await api("GET", `/api/users/${eva.id}/identities`, admin);
      check(ids.length === 1 && ids[0].subject === "eva-1" && ids[0].issuer === idp.issuer, "linked to her provider sign-in");

      // Eva may read the master and nothing more: its entries are there
      // to read, and nothing that changes them is offered.
      await api("PUT", "/api/grants", admin, { userId: eva.id, kind: "termbase", resource: "E2E Recht", role: "read" });
      await open(`/terminology/srv:${encodeURIComponent("E2E Recht")}`, { keep: true });
      check(await waitFor("document.querySelector('.pane-entries .pane-sub')?.innerText.includes('Read')", 15000), "a reader is told what she may do");
      check(
        await evaluate("![...document.querySelectorAll('.pane-tools button')].some(b => /Import|Settings/.test(b.textContent)) && [...document.querySelectorAll('.pane-tools button')].some(b => b.textContent.includes('Export'))"),
        "a reader exports, and neither imports nor changes the settings",
      );
      check(await evaluate("!document.querySelector('.pane-entries').innerText.includes('New entry') && ![...document.querySelectorAll('.pane-entries [role=tab]')].some(t => t.textContent.includes('Duplicates'))"), "a reader adds no entries and merges no duplicates");
      check(await waitFor("document.querySelector('#panel-entry')?.innerText.includes('Approved')", 10000), "an entry is shown to read");
      check(await evaluate("!document.querySelector('.pane-detail').innerText.includes('Delete entry') && !document.querySelector('#panel-entry select, #panel-entry input')"), "and offers nothing to change");

      // Nothing from the server is kept in the browser.
      await open("/projects", { keep: true });
      check(
        await waitFor("document.querySelector('main').innerText.includes('On the team server')"),
        "after reloading, the sign-in is kept",
      );
      const local = await evaluate(
        "new Promise(r => { const q = indexedDB.open('interlinea-prototype', 1); q.onsuccess = () => { const g = q.result.transaction('kv').objectStore('kv').get('state'); g.onsuccess = () => r(JSON.stringify((g.result?.state?.projects ?? []).map(p => p.id))); g.onerror = () => r('error'); }; q.onerror = () => r('error'); })",
      );
      check(!local.includes("srv-"), "server projects are not saved in the browser: " + local);
      const evaKept = JSON.parse((await evaluate("localStorage.getItem('interlinea.server')")) ?? "null");
      check(evaKept?.cookie === true && !evaKept.token, "a provider sign-in keeps no token in the browser either");

      // Access for programs: a token made in the settings signs an
      // assistant in on the MCP endpoint, and is refused once it is ended.
      {
        await open("/settings", { keep: true, wait: 0, reload: false });
        const panel = "section[aria-labelledby=s-programs]";
        check(await waitFor(`!!document.querySelector(${JSON.stringify(panel)})`, 10000), "signed in to a server, the settings offer access for programs");
        check((await evaluate(`document.querySelector(${JSON.stringify(panel)}).innerText`)).includes("/mcp"), "with the address assistants connect to");
        await setValue(`${panel} form input`, "My assistant");
        await sleep(100);
        await click(`${panel} button`, "Make a token");
        check(await waitFor("!!document.querySelector('.token-value')", 10000), "the token is shown once");
        const token = await evaluate("document.querySelector('.token-value').textContent");
        const mcp = (tok, method, params) => fetch(`${base}/mcp`, { method: "POST", headers: { Authorization: `Bearer ${tok}`, "Content-Type": "application/json" }, body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }) });
        let r = await mcp(token, "tools/list");
        const tools = r.ok ? (await r.json()).result.tools.map((t) => t.name) : [];
        check(tools.includes("list_segments") && tools.includes("save_translation"), "the token signs in on the MCP endpoint: " + tools.join(", "));
        r = await mcp(token, "tools/call", { name: "list_projects", arguments: {} });
        check(r.ok && !(await r.json()).result.isError, "and lists the person's projects");
        check((await evaluate(`document.querySelector(${JSON.stringify(panel + " .settings-list")}).innerText`)).includes("My assistant"), "the token is listed by the program's name");
        await click(".token-made button", "Done");
        await evaluate(`[...document.querySelectorAll(${JSON.stringify(panel + " .settings-list li")})].find(li => li.textContent.includes('My assistant')).querySelector('button').click()`);
        await sleep(150);
        await click(`${panel} .confirm button`, "End");
        check(await waitFor(`!document.querySelector(${JSON.stringify(panel + " .settings-list")}).innerText.includes('My assistant')`, 10000), "ended, it is gone from the list");
        check((await mcp(token, "tools/list")).status === 401, "and refused on the endpoint");
        check((await evaluate("document.querySelector('section[aria-labelledby=s-mt]').innerText")).includes("An administrator adds the services of the server") || (await evaluate("document.querySelector('section[aria-labelledby=s-mt]').innerText")).includes("Add a service"), "the settings show the server's translation services");
      }
    },
    { teamServer: true },
  );
