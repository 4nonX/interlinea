// Every document format goes from the file to the translated file:
// PowerPoint, Excel, a web page and InDesign IDML in one project, and a
// memoQ handoff package to its delivery package.
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { inflateRawSync } from "node:zlib";
import { run } from "./harness.mjs";

/** The entries of a ZIP file, by name. */
function unzip(buf) {
  const out = {};
  let p = buf.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  const n = buf.readUInt16LE(p + 10);
  p = buf.readUInt32LE(p + 16);
  for (let i = 0; i < n; i++) {
    const method = buf.readUInt16LE(p + 10);
    const size = buf.readUInt32LE(p + 20);
    const nameLen = buf.readUInt16LE(p + 28);
    const extra = buf.readUInt16LE(p + 30);
    const comment = buf.readUInt16LE(p + 32);
    const local = buf.readUInt32LE(p + 42);
    const name = buf.toString("utf8", p + 46, p + 46 + nameLen);
    const start = local + 30 + buf.readUInt16LE(local + 26) + buf.readUInt16LE(local + 28);
    const raw = buf.subarray(start, start + size);
    out[name] = method === 0 ? raw : inflateRawSync(raw);
    p += 46 + nameLen + extra + comment;
  }
  return out;
}

export default () =>
  run(
    "Document formats and memoQ packages",
    async ({ send, evaluate, click, text, check, sleep, open, setValue, key, fixtures, downloads, waitFor, errors }) => {
      const docs = `${fixtures}/documents`;
      const setFiles = async (files) => {
        const doc = await send("DOM.getDocument", { depth: -1, pierce: true });
        const q = await send("DOM.querySelector", { nodeId: doc.root.nodeId, selector: "input[type=file][multiple]:not([webkitdirectory])" });
        await send("DOM.setFileInputFiles", { nodeId: q.nodeId, files });
      };
      const through = async () => {
        await click("button", "Continue to memories");
        await sleep(300);
        await click("button", "Continue to terminology");
        await sleep(300);
        await click("button", "Continue to review");
        await waitFor("!!document.querySelector('.analysis')");
      };
      /** Opens a file of the project, types a translation into its named segment and confirms it. */
      const translate = async (file, source, translation) => {
        await setValue("#ed-file", await evaluate(`[...document.querySelectorAll('#ed-file option')].find(o => o.textContent.includes(${JSON.stringify(file)})).value`));
        await waitFor(`document.querySelector('#ed-file option:checked')?.textContent.includes(${JSON.stringify(file)}) && document.querySelectorAll('.seg').length > 0`);
        await sleep(400);
        const found = await evaluate(`(() => { const s = [...document.querySelectorAll('.seg')].find(s => s.querySelector('.seg-source').innerText.replace(/\\s+/g, ' ').includes(${JSON.stringify(source)})); if (!s) return false; (s.querySelector('.seg-target-view') ?? s.querySelector('.target-editor')).click(); return true; })()`);
        if (!found) return false;
        await sleep(400);
        await evaluate("document.querySelector('.seg.is-active .target-editor')?.focus()");
        await sleep(200);
        await key("a", "KeyA", 65, 2);
        await send("Input.insertText", { text: translation });
        await sleep(200);
        await key("Enter", "Enter", 13, 2);
        await sleep(400);
        return true;
      };
      const save = async (label, name, said = name) => {
        await click("button", label);
        const ok = await waitFor(`document.body.innerText.includes(${JSON.stringify("Saved " + said)})`);
        await sleep(500);
        return ok && readdirSync(downloads).includes(name);
      };

      await open("/new", { wait: 0 });
      await waitFor("[...document.querySelectorAll('button')].some(b => b.textContent.includes('Continue to files and languages'))", 15000);
      await click("button", "Continue to files and languages");
      await sleep(300);
      await setFiles([`${docs}/Folien.pptx`, `${docs}/Tabelle.xlsx`, `${docs}/Seite.html`, `${docs}/Layout.idml`, `${docs}/Vertrag.pdf`]);
      check(await waitFor("document.querySelectorAll('.file-table tbody tr').length === 5", 15000), "the five files are listed");
      check(
        await evaluate("[...document.querySelectorAll('.file-table tbody tr')].every(r => r.querySelector('select').value === 'translatable' && !r.textContent.includes('reference file'))"),
        "a presentation, a workbook, a web page, an InDesign document and a PDF are files to translate",
      );
      {
        const row = await evaluate("[...document.querySelectorAll('.file-table tbody tr')].find(r => r.textContent.includes('Vertrag.pdf')).innerText");
        check(row.includes("PDF document, text only, saved as a DOCX document"), "the PDF says what is kept of it: " + row.replace(/\s+/g, " ").slice(0, 200));
        check(row.includes("The text of a PDF is read into a DOCX document"), "and what becomes of it");
        if (process.env.INTERLINEA_SHOTS) {
          await evaluate("[...document.querySelectorAll('.file-table tbody tr')].find(r => r.textContent.includes('Vertrag.pdf')).scrollIntoView({ block: 'center' })");
          await sleep(200);
          const shot = await send("Page.captureScreenshot", { format: "png" });
          writeFileSync(`${process.env.INTERLINEA_SHOTS}/pdf-files.png`, Buffer.from(shot.data, "base64"));
        }
      }
      check((await text()).includes("Detected German"), "the language is read from their text");
      await setValue("#pname", "Vier Formate");
      await sleep(200);
      await through();
      check((await text()).includes("5 files to translate"), "the review counts them");
      await click("button", "Create project");
      check(await waitFor("!!document.querySelector('#ed-file')"), "the editor offers the files");

      // A presentation: slides in the order of the show, notes after their slide.
      check(await translate("Folien.pptx", "Mietrecht 2027", "Tenancy law 2027"), "the presentation's title is a segment");
      let t = await text();
      check(t.includes("Begrüßung.") && t.includes("Nettokaltmiete") && !t.includes("Titelmasterformat"), "notes and chart labels are read, the master's prompt is not");
      check(await save("Save translated document", "Folien_en-GB.pptx"), "the presentation is saved under its name in the target language");
      let z = unzip(readFileSync(`${downloads}/Folien_en-GB.pptx`));
      check(z["ppt/slides/slide2.xml"].toString().includes("<a:t>Tenancy law 2027</a:t>"), "the slide has the translation");

      // A workbook: cells, not numbers.
      check(await translate("Tabelle.xlsx", "Kündigung durch den Mieter", "Notice by the tenant"), "a cell of the workbook is a segment");
      check((await evaluate("document.querySelectorAll('.seg').length")) === 4, "the workbook's four texts, without its number");
      check(await save("Save translated document", "Tabelle_en-GB.xlsx"), "the workbook is saved");
      z = unzip(readFileSync(`${downloads}/Tabelle_en-GB.xlsx`));
      check(
        z["xl/sharedStrings.xml"].toString().includes("<si><t>Notice by the tenant</t></si>") && z["xl/worksheets/sheet1.xml"].toString().includes('<c r="B2"><v>3</v></c>'),
        "the cell has the translation and the number is as it was",
      );

      // A web page: the title, the text with its tags, the picture's description.
      check(await translate("Seite.html", "Das Haus in der Lindenstraße", "The house in Lindenstraße"), "a picture's description is a segment");
      check(
        (await evaluate("[...document.querySelectorAll('.seg')].find(s => s.querySelector('.seg-source').innerText.includes('Die Kaution'))?.querySelectorAll('.seg-source .tag-chip').length")) === 2,
        "bold text of the page is a pair of tags",
      );
      check(await save("Save translated document", "Seite_en-GB.html"), "the page is saved");
      const page = readFileSync(`${downloads}/Seite_en-GB.html`, "utf8");
      check(
        page.includes('<html lang="en-GB">') && page.includes('alt="The house in Lindenstraße"') && page.includes("<p>Die Kaution ist <b>fällig</b>."),
        "the page has the translation, the target language, and its other text as it was",
      );

      // An InDesign document.
      check(await translate("Layout.idml", "Näheres steht im Mietvertrag.", "The lease has the details."), "a paragraph of the InDesign document is a segment");
      check(await save("Save translated document", "Layout_en-GB.idml"), "the InDesign document is saved");
      z = unzip(readFileSync(`${downloads}/Layout_en-GB.idml`));
      check(
        Object.keys(z)[0] === "mimetype" && z["Stories/Story_u1.xml"].toString().includes("<Content>The lease has the details.</Content>"),
        "the story has the translation and the package its type first",
      );
      // A PDF: its text in paragraphs, the two columns one after the other,
      // and its translation a Word document.
      check(await translate("Vertrag.pdf", "Die Kaution beträgt drei Monatsmieten.", "The deposit is three months' rent."), "a paragraph of the PDF is a segment");
      {
        const sources = await evaluate("[...document.querySelectorAll('.seg .seg-source')].map(s => s.innerText.replace(/\\s+/g, ' ').trim())");
        const at = (t) => sources.findIndex((x) => x.includes(t));
        check(sources[0] === "Mietvertrag über Wohnraum" && at("Ruhestörender Lärm ist zu vermeiden") > 0 && at("Ruhestörender Lärm ist zu vermeiden") < at("Haustiere dürfen nur mit vorheriger Zustimmung"), "the lines of a column are joined to sentences, the left column before the right: " + sources.length);
        check(await save("Save translated document", "Vertrag_en-GB.docx"), "the translation of the PDF is saved as a Word document");
        const doc = unzip(readFileSync(`${downloads}/Vertrag_en-GB.docx`))["word/document.xml"].toString();
        check(doc.includes(">The deposit is three months' rent.</w:t>") && doc.includes("Mietvertrag über Wohnraum"), "it has the translation, and the other text as it was");
      }

      // The preview shows the workbook's sheet as a table.
      await setValue("#ed-file", await evaluate("[...document.querySelectorAll('#ed-file option')].find(o => o.textContent.includes('Tabelle.xlsx')).value"));
      await sleep(500);
      await click("button", "Preview");
      check(await waitFor("document.querySelectorAll('.preview .pv-table td').length >= 4"), "the preview lays the workbook's cells out as its sheet");
      await click("button", "Preview");

      // A memoQ handoff package becomes a project, and goes back as a delivery package.
      await open("/new", { keep: true, wait: 0 });
      await waitFor("[...document.querySelectorAll('button')].some(b => b.textContent.includes('Continue to files and languages'))", 15000);
      await click("button", "Continue to files and languages");
      await sleep(300);
      await setFiles([`${docs}/Hofmann.mqout`]);
      check(await waitFor("document.body.innerText.includes('From the handoff package Hofmann.mqout')", 15000), "the package's document is listed, out of its archive");
      t = await text();
      check(t.includes("Mietvertrag.docx.mqxliff") && t.includes("1 of 3 segments confirmed"), "with its name and how far it is: " + (t.match(/From the handoff[^\n]*/)?.[0] ?? ""));
      await setValue("#pname", "Hofmann memoQ");
      await sleep(200);
      await through();
      check((await text()).includes("from the handoff package Hofmann.mqout"), "the review names the package");
      await click("button", "Create project");
      check(await waitFor("document.querySelectorAll('.seg').length === 3"), "the document opens with its three segments");
      await evaluate("document.querySelector('.seg.is-active .target-editor')?.focus()");
      await sleep(200);
      await key("a", "KeyA", 65, 2);
      await send("Input.insertText", { text: "Made in Interlinea" });
      await sleep(200);
      await key("Enter", "Enter", 13, 2);
      await sleep(300);
      check(await save("Save return package", "Hofmann.mqback", "the return package Hofmann.mqback"), "the delivery package is saved");
      z = unzip(readFileSync(`${downloads}/Hofmann.mqback`));
      check(Object.keys(z).sort().join(",") === "DeliveryPackageInfo.xml,Mietvertrag.docx.mqxliff", "it holds the document and the delivery's description: " + Object.keys(z).join(","));
      check(
        z["Mietvertrag.docx.mqxliff"].toString().includes("Made in Interlinea") && z["DeliveryPackageInfo.xml"].toString().includes("<HandoffDeliveryPackageInfo") && z["DeliveryPackageInfo.xml"].toString().includes("<User>Anna Lindner</User>"),
        "the document has the translation; the description is the handoff's under the delivery's names",
      );

      // A memoQ XLIFF archive on its own is a bilingual file and goes back into its archive.
      await open("/new", { keep: true, wait: 0 });
      await waitFor("[...document.querySelectorAll('button')].some(b => b.textContent.includes('Continue to files and languages'))", 15000);
      await click("button", "Continue to files and languages");
      await sleep(300);
      await setFiles([`${docs}/Mietvertrag.mqxlz`]);
      check(await waitFor("document.body.innerText.includes('.mqxliff')", 15000), "an archive is read as memoQ XLIFF");
      await setValue("#pname", "Archiv");
      await sleep(200);
      await through();
      await click("button", "Create project");
      await waitFor("document.querySelectorAll('.seg').length === 3");
      await evaluate("document.querySelector('.seg.is-active .target-editor')?.focus()");
      await sleep(200);
      await key("a", "KeyA", 65, 2);
      await send("Input.insertText", { text: "Back into the archive" });
      await sleep(200);
      await key("Enter", "Enter", 13, 2);
      await sleep(300);
      check(await save("Save XLIFF", "Mietvertrag.mqxlz"), "the archive is saved under its name");
      z = unzip(readFileSync(`${downloads}/Mietvertrag.mqxlz`));
      check(z["document.mqxliff"].toString().includes("Back into the archive") && !!z["skeleton.xml"], "the document in it has the translation, and what memoQ needs is still there");
      // The strings of software, Markdown and subtitles: read by their
      // filters, with keys, markup and times kept.
      await open("/new", { keep: true, wait: 0 });
      await waitFor("[...document.querySelectorAll('button')].some(b => b.textContent.includes('Continue to files and languages'))", 15000);
      await click("button", "Continue to files and languages");
      await sleep(300);
      await setFiles([`${docs}/app.properties`, `${docs}/LIESMICH.md`, `${docs}/film.srt`]);
      check(await waitFor("document.querySelectorAll('.file-table tbody tr').length === 3", 15000), "a properties file, a Markdown file and subtitles are listed");
      t = await text();
      check(t.includes("Properties file, keys and placeholders kept") && t.includes("Markdown file, markup kept") && t.includes("SRT subtitles, times kept"), "each is named with what it keeps");
      check(await evaluate("[...document.querySelectorAll('.file-table tbody tr')].every(r => r.querySelector('select').value === 'translatable')"), "all three are files to translate");
      await setValue("#pname", "Software und Untertitel");
      await sleep(200);
      await through();
      await click("button", "Create project");
      check(await waitFor("!!document.querySelector('#ed-file')"), "the editor offers them");
      check(await translate("app.properties", "Auf Wiedersehen", "Goodbye"), "a string of the properties file is a segment");
      check((await evaluate("document.querySelectorAll('.seg').length")) === 2 && (await evaluate("!!document.querySelector('.seg .tag-chip')")), "its two strings, the placeholder as a tag");
      check(await save("Save translated document", "app_en-GB.properties"), "the properties file is saved");
      check(readFileSync(`${downloads}/app_en-GB.properties`, "utf8") === "# Start page\ngreeting=Guten Tag, {0}!\nbye=Goodbye\n", "with the translation, and everything else as it was");
      check(await translate("LIESMICH.md", "Der Vermieter haftet nicht.", "The landlord is not liable."), "a sentence of the Markdown file is a segment");
      check(!(await text()).includes("nicht übersetzen"), "its code block is not read");
      check(await save("Save translated document", "LIESMICH_en-GB.md"), "the Markdown file is saved");
      check(readFileSync(`${downloads}/LIESMICH_en-GB.md`, "utf8") === "# Mietvertrag\n\nDer **Mieter** zahlt die Miete. The landlord is not liable.\n\n```\nnicht übersetzen\n```\n", "with the translation in its place");
      check(await translate("film.srt", "Auf Wiedersehen.", "Goodbye."), "a subtitle is a segment");
      check(await save("Save translated document", "film_en-GB.srt"), "the subtitles are saved");
      check(readFileSync(`${downloads}/film_en-GB.srt`, "utf8").endsWith("2\n00:00:04,000 --> 00:00:05,000\nGoodbye.\n"), "with their numbers and times");
      // Segments are joined and split by the translator; the file is
      // written with what was translated where its sentences stood.
      {
        const pick = async (source) => {
          await evaluate(`(() => { const s = [...document.querySelectorAll('.seg')].find(s => s.querySelector('.seg-source').innerText.replace(/\\s+/g, ' ').includes(${JSON.stringify(source)})); (s.querySelector('.seg-target-view') ?? s.querySelector('.target-editor')).click(); })()`);
          await sleep(400);
        };
        const button = (label) => `[...document.querySelectorAll('.tool-row button')].find(b => b.textContent.includes(${JSON.stringify(label)}))`;
        const count = () => evaluate("document.querySelectorAll('.seg').length");
        const activeSource = () => evaluate("document.querySelector('.seg.is-active .seg-source').innerText.replace(/\\s+/g, ' ').trim()");
        await setValue("#ed-file", await evaluate(`[...document.querySelectorAll('#ed-file option')].find(o => o.textContent.includes("app.properties")).value`));
        await waitFor("document.querySelectorAll('.seg').length === 2");
        await pick("Auf Wiedersehen");
        check(await evaluate(`${button("Join with next")}.disabled && !${button("Split")}.disabled`), "strings of a properties file are not joined, each is its own paragraph");
        await setValue("#ed-file", await evaluate(`[...document.querySelectorAll('#ed-file option')].find(o => o.textContent.includes("LIESMICH.md")).value`));
        await waitFor("document.querySelector('#ed-file option:checked')?.textContent.includes('LIESMICH.md') && document.querySelectorAll('.seg').length === 3");
        await pick("Mietvertrag");
        check(await evaluate(`${button("Join with next")}.disabled`), "a heading is not joined with the paragraph after it");
        await pick("zahlt die Miete");
        await evaluate(`${button("Join with next")}.click()`);
        check(await waitFor("document.querySelectorAll('.seg').length === 2", 5000), "two sentences of a paragraph are joined");
        check((await activeSource()).endsWith("zahlt die Miete. Der Vermieter haftet nicht.") && (await evaluate("!!document.querySelector('.seg.is-active .target-editor')")), "the joined segment is the open one, with both sentences: " + (await activeSource()));
        await waitFor("!!document.querySelector('.seg.is-active .target-editor')", 5000);
        check((await evaluate("document.querySelector('.seg.is-active .target-editor').innerText.trim()")) === "The landlord is not liable." && (await text()).includes("Joined from 2 segments of the file."), "it has the translation there was, and says what it is");
        await evaluate("document.querySelector('.seg.is-active .target-editor').focus()");
        await key("a", "KeyA", 65, 2);
        await send("Input.insertText", { text: "The landlord is not liable, and the tenant pays." });
        await sleep(300);
        check(await save("Save translated document", "LIESMICH_en-GB.md"), "the file is saved with the joined segment");
        check(readFileSync(`${downloads}/LIESMICH_en-GB.md`, "utf8") === "# Mietvertrag\n\nThe landlord is not liable, and the tenant pays.\n\n```\nnicht übersetzen\n```\n", "one translation stands where the two sentences stood: " + JSON.stringify(readFileSync(`${downloads}/LIESMICH_en-GB.md`, "utf8")));
        if (process.env.INTERLINEA_SHOTS) {
          const shot = await send("Page.captureScreenshot", { format: "png" });
          writeFileSync(`${process.env.INTERLINEA_SHOTS}/joined-segment.png`, Buffer.from(shot.data, "base64"));
        }
        // What was joined is split anywhere: inside the second sentence, the parts stand for both.
        const caretAt = (words) => evaluate(`(() => { const el = document.querySelector('.split-source'); const n = [...el.childNodes].find(n => n.nodeType === 3 && n.textContent.includes(${JSON.stringify(words)})); const r = document.createRange(); r.setStart(n, n.textContent.indexOf(${JSON.stringify(words)})); r.collapse(true); const s = getSelection(); s.removeAllRanges(); s.addRange(r); })()`);
        const splitHere = async (words) => {
          await evaluate(`${button("Split")}.click()`);
          await waitFor("!!document.querySelector('.split-source')", 5000);
          await caretAt(words);
          await waitFor("![...document.querySelectorAll('dialog[open] button')].find(b => b.textContent.trim() === 'Split').disabled", 3000);
          await evaluate("[...document.querySelectorAll('dialog[open] button')].find(b => b.textContent.trim() === 'Split').click()");
          await waitFor("!document.querySelector('.split-source')", 5000);
          await sleep(300);
        };
        check(await evaluate(`!${button("Split")}.disabled`), "a joined segment can be split");
        await splitHere("haftet nicht");
        check((await count()) === 3 && (await activeSource()).endsWith("zahlt die Miete. Der Vermieter") && (await text()).includes("A part of 2 segments of the file that were joined."), "split inside its second sentence, the parts say what they are: " + (await activeSource()));
        await pick("haftet nicht.");
        await evaluate("document.querySelector('.seg.is-active .target-editor').focus()");
        await send("Input.insertText", { text: "No liability." });
        await sleep(300);
        check(await save("Save translated document", "LIESMICH_en-GB.md"), "the file is saved with a joined stretch in parts");
        check(readFileSync(`${downloads}/LIESMICH_en-GB.md`, "utf8").includes("\n\nThe landlord is not liable, and the tenant pays. No liability.\n\n"), "the parts stand one after the other where the two sentences stood: " + JSON.stringify(readFileSync(`${downloads}/LIESMICH_en-GB.md`, "utf8")));
        // Joined again and split where the sentences met, they stand apart as the file has them.
        await pick("zahlt die Miete");
        await evaluate(`${button("Join with next")}.click()`);
        check(await waitFor("document.querySelectorAll('.seg').length === 2", 5000), "the parts are joined again");
        await waitFor("!!document.querySelector('.seg.is-active .target-editor')", 5000);
        await splitHere("Der Vermieter haftet");
        check((await count()) === 3 && (await activeSource()).endsWith("zahlt die Miete.") && !(await text()).includes("segments of the file"), "split where they met, the two sentences are the file's again: " + (await activeSource()));
        // A sentence cleared for the next step.
        await pick("Der Vermieter haftet nicht.");
        await waitFor("!!document.querySelector('.seg.is-active .target-editor')", 5000);
        await evaluate("document.querySelector('.seg.is-active .target-editor').focus()");
        await key("a", "KeyA", 65, 2);
        await send("Input.insertText", { text: "The landlord is not liable." });
        await sleep(300);
        // Split: the caret in the source is the place.
        await pick("Der Vermieter haftet nicht.");
        await evaluate(`${button("Split")}.click()`);
        check(await waitFor("!!document.querySelector('.split-source')", 5000) && (await evaluate("document.activeElement?.classList.contains('split-source')")), "the split dialog shows the source, with the caret in it");
        check(await evaluate("[...document.querySelectorAll('dialog[open] button')].find(b => b.textContent.trim() === 'Split').disabled"), "at the start there is nothing to split");
        await evaluate("(() => { const el = document.querySelector('.split-source'); const r = document.createRange(); r.setStart(el.firstChild, 14); r.collapse(true); const s = getSelection(); s.removeAllRanges(); s.addRange(r); })()");
        check(await waitFor("document.querySelector('.split-parts')?.innerText.replace(/\\s+/g, ' ').includes('First part Der Vermieter Second part haftet nicht.')", 3000), "the two parts are shown: " + (await evaluate("document.querySelector('.split-parts').innerText")));
        if (process.env.INTERLINEA_SHOTS) {
          const shot = await send("Page.captureScreenshot", { format: "png" });
          writeFileSync(`${process.env.INTERLINEA_SHOTS}/split-dialog.png`, Buffer.from(shot.data, "base64"));
        }
        await send("Input.insertText", { text: "x" });
        await sleep(150);
        check((await evaluate("document.querySelector('.split-source').innerText")) === "Der Vermieter haftet nicht.", "the source is not changed by typing");
        await evaluate("[...document.querySelectorAll('dialog[open] button')].find(b => b.textContent.trim() === 'Split').click()");
        check(await waitFor("document.querySelectorAll('.seg').length === 4 && !document.querySelector('.split-source')", 5000), "split, there is one segment more");
        await waitFor("!!document.querySelector('.seg.is-active .target-editor')", 5000);
        check((await activeSource()) === "Der Vermieter" && (await text()).includes("A part of a segment of the file that was split."), "the first part is the open one: " + (await activeSource()));
        check((await evaluate("document.querySelector('.seg.is-active .target-editor').innerText.trim()")) === "The landlord is not liable.", "and keeps the translation the segment had");
        await pick("haftet nicht.");
        await evaluate("document.querySelector('.seg.is-active .target-editor').focus()");
        await send("Input.insertText", { text: "is not liable." });
        await sleep(300);
        check(await save("Save translated document", "LIESMICH_en-GB.md"), "the file is saved with the split segment");
        check(readFileSync(`${downloads}/LIESMICH_en-GB.md`, "utf8").includes("No liability. The landlord is not liable. is not liable.\n"), "the parts' translations stand one after the other: " + JSON.stringify(readFileSync(`${downloads}/LIESMICH_en-GB.md`, "utf8")));
        await pick("Der Vermieter");
        await evaluate("document.querySelector('.seg.is-active .target-editor').focus()");
        await key("j", "KeyJ", 74, 9);
        check(await waitFor("document.querySelectorAll('.seg').length === 3", 5000) && (await activeSource()) === "Der Vermieter haftet nicht.", "Alt+Shift+J joins the parts into the segment it was");
        await waitFor("!!document.querySelector('.seg.is-active .target-editor')", 5000);
        check((await evaluate("document.querySelector('.seg.is-active .target-editor').innerText.trim()")) === "The landlord is not liable. is not liable.", "with what was translated of its parts");
      }
      // A project's own segmentation rules cut its documents.
      await open("/new", { keep: true, wait: 0 });
      await waitFor("[...document.querySelectorAll('button')].some(b => b.textContent.includes('Continue to files and languages'))", 15000);
      await click("button", "Continue to files and languages");
      await sleep(300);
      await setFiles([`${docs}/LIESMICH.md`]);
      await waitFor("document.querySelectorAll('.file-table tbody tr').length === 1", 15000);
      await setValue("#pname", "Eigene Regeln");
      await sleep(200);
      await through();
      await evaluate("[...document.querySelectorAll('details.settings-details')].find(d => d.querySelector('summary').textContent.includes('Segmentation')).open = true");
      check((await text()).includes("Segmentation as usual for the language"), "the review says the documents are cut as usual");
      await click("details.settings-details button", "Add a segmentation rule");
      await sleep(200);
      check((await text()).includes("Say what stands before the place, after it, or both."), "a rule that says nothing is a problem");
      await setValue("#seg-rule-0-before", "(");
      await sleep(400);
      check((await text()).includes("not a regular expression that can be used"), "and so is one that is no regular expression");
      await setValue("#seg-rule-0-before", "Miete\\.");
      await setValue("#seg-rule-0-after", "\\s");
      await sleep(500);
      check((await text()).includes("1 rule of your own") && !(await text()).includes("Say what stands before"), "filled in, the rule is counted");
      await click("button", "Create project");
      check(await waitFor("document.querySelectorAll('.seg').length === 2", 15000), "the two sentences the rule keeps together are one segment");
      check((await evaluate("document.querySelectorAll('.seg')[1].querySelector('.seg-source').innerText.replace(/\\s+/g, ' ')")).endsWith("zahlt die Miete. Der Vermieter haftet nicht."), "with both sentences");
      await open("/projects", { keep: true, wait: 0 });
      await waitFor("[...document.querySelectorAll('a')].some(a => a.textContent.includes('Eigene Regeln'))", 10000);
      await evaluate("location.hash = '#/project/' + JSON.parse(JSON.stringify([...document.querySelectorAll('a')].find(a => a.textContent.includes('Eigene Regeln')).getAttribute('href'))).split('/').pop()");
      check(await waitFor("!!document.querySelector('#seg-rule-0-before') && document.querySelector('#seg-rule-0-before').value === 'Miete\\\\.'", 10000), "the project's settings show its rule: " + (await evaluate("location.hash + ' ' + (document.querySelector('#seg-rule-0-before')?.value ?? 'no field')")));
      check((await text()).includes("They cut the documents added from now on"), "and say it is for files added from now on");
      check(errors.length === 0, "no console errors: " + errors.join(" | "));
    },
  );
