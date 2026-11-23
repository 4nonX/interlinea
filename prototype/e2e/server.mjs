// The web app on a real team server: signing in, making a project on the
// server from the New project flow, giving work to a translator, the
// translator seeing only their part, translations saved on the server,
// others' changes and presence arriving live, and reporting work done.
import { run, call, ADMIN_PASSWORD } from "./harness.mjs";

export default () =>
  run(
    "Team server",
    async ({ base, send, evaluate, click, text, body, check, sleep, open, setValue, key, samples, waitFor }) => {
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
    },
    { teamServer: true },
  );
