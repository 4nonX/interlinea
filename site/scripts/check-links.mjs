// Checks every link and picture of the built site that stays on the site:
// the page or file it names must exist, and an #anchor must be an id on
// that page. Links to other sites are not followed.
//
//   node scripts/check-links.mjs dist
import { readdir, readFile, stat } from 'node:fs/promises';
import { join, relative, dirname, posix } from 'node:path';

const root = process.argv[2] || 'dist';
const pages = new Map();

async function walk(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const p = join(dir, entry.name);
    if (entry.isDirectory()) await walk(p);
    else if (entry.name.endsWith('.html')) pages.set(p, await readFile(p, 'utf8'));
  }
}

async function exists(p) {
  try {
    return await stat(p);
  } catch {
    return null;
  }
}

// The file a site path is served from, as GitHub Pages serves it.
async function resolveFile(sitePath) {
  const clean = decodeURIComponent(sitePath.split('?')[0]);
  const p = join(root, clean);
  const s = await exists(p);
  if (s && s.isFile()) return p;
  if (s && s.isDirectory() && (await exists(join(p, 'index.html')))) return join(p, 'index.html');
  if (await exists(`${p}.html`)) return `${p}.html`;
  return null;
}

function ids(html) {
  return new Set([...html.matchAll(/\sid="([^"]+)"/g)].map((m) => m[1]));
}

await walk(root);
const problems = [];
for (const [file, html] of pages) {
  const here = '/' + relative(root, dirname(file)).split('\\').join('/');
  // the browser client under /app is copied from the release and checked there
  if (here.startsWith('/app')) continue;
  const refs = [...html.matchAll(/\s(?:href|src)="([^"]+)"/g)].map((m) => m[1]);
  for (const ref of refs) {
    if (/^(?:[a-z][a-z0-9+.-]*:|\/\/)/i.test(ref)) continue;
    const [path, anchor] = ref.split('#');
    let target = file;
    if (path) {
      const abs = path.startsWith('/') ? path : posix.join(here, path);
      // /app is filled in by the workflow from the release
      if (abs === '/app/' || abs.startsWith('/app/')) continue;
      target = await resolveFile(abs);
      if (!target) {
        problems.push(`${relative(root, file)}: ${ref} does not exist`);
        continue;
      }
    }
    if (anchor && target.endsWith('.html')) {
      const targetHtml = pages.get(target) ?? (await readFile(target, 'utf8'));
      if (!ids(targetHtml).has(decodeURIComponent(anchor))) problems.push(`${relative(root, file)}: ${ref} has no #${anchor} there`);
    }
  }
}

if (problems.length) {
  console.error(problems.join('\n'));
  console.error(`check-links: ${problems.length} broken in ${pages.size} pages`);
  process.exit(1);
}
console.log(`check-links: ${pages.size} pages, every link on the site leads somewhere`);
