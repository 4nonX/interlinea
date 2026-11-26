// Copies the manual in ../docs into the site's docs collection, so the
// repository's docs folder stays the one source. Each file's first heading
// becomes its title, links between the files point at their routes, images
// point at the copies in public/docs-img, and links to other files of the
// repository point at GitHub.
import { mkdir, readdir, readFile, rm, writeFile, copyFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const site = join(here, '..');
const repo = join(site, '..');
const docs = join(repo, 'docs');
const out = join(site, 'src', 'content', 'docs', 'docs', 'manual');
const images = join(site, 'public', 'docs-img');
const github = 'https://github.com/4nonX/interlinea';

// The order and the short sidebar labels of the manual. A file in docs/
// that is not listed here is still copied and comes last.
export const manual = [
  ['guide', 'Using Interlinea'],
  ['formats', 'File formats'],
  ['matching', 'Matches, terms and checks'],
  ['languages', 'Languages'],
  ['cli', 'Command line'],
  ['running', 'Running a team server'],
  ['server', 'Team server'],
  ['integration', 'Translation services and assistants'],
  ['api', 'API reference'],
  ['development', 'Development'],
  ['desktop', 'Desktop app internals'],
];

function rewriteTarget(target, names) {
  if (/^[a-z][a-z0-9+.-]*:/i.test(target) || target.startsWith('#')) return target;
  const [path, hash = ''] = target.split('#');
  const anchor = hash ? `#${hash}` : '';
  const page = path.match(/^(?:\.\/)?([a-z0-9-]+)\.md$/i);
  if (page && names.has(page[1])) return `/docs/manual/${page[1]}/${anchor}`;
  const image = path.match(/^(?:\.\/)?screenshots\/([^/]+)$/);
  if (image) return `/docs-img/${image[1]}`;
  const up = path.match(/^\.\.\/(.+)$/);
  if (up) {
    const rel = up[1].replace(/\/$/, '');
    const kind = existsSync(join(repo, rel)) && !/\.[a-z0-9]+$/i.test(rel) ? 'tree' : 'blob';
    return `${github}/${kind}/main/${rel}${anchor}`;
  }
  return `${github}/blob/main/docs/${path}${anchor}`;
}

function convert(text, name, names) {
  const lines = text.replace(/\r\n/g, '\n').split('\n');
  let title = name;
  const first = lines.findIndex((l) => l.trim() !== '');
  if (first >= 0 && lines[first].startsWith('# ')) {
    title = lines[first].slice(2).trim();
    lines.splice(0, first + 1);
  }
  let fence = false;
  const body = lines.map((line) => {
    if (/^\s*(```|~~~)/.test(line)) fence = !fence;
    if (fence) return line;
    return line
      .replace(/(!?\[[^\]]*\])\(([^)\s]+)((?:\s+"[^"]*")?)\)/g, (_, label, target, t) => `${label}(${rewriteTarget(target, names)}${t})`)
      .replace(/(<img\b[^>]*\bsrc=")([^"]+)(")/g, (_, a, target, b) => `${a}${rewriteTarget(target, names)}${b}`);
  }).join('\n');
  const front = [
    '---',
    `title: ${JSON.stringify(title)}`,
    `editUrl: ${JSON.stringify(`${github}/edit/main/docs/${name}.md`)}`,
    '---',
    '',
    '<!-- Copied from docs/' + name + '.md by site/scripts/sync-docs.mjs. Edit the original. -->',
    '',
  ].join('\n');
  return front + body.replace(/^\n+/, '');
}

async function main() {
  const files = (await readdir(docs)).filter((f) => f.endsWith('.md'));
  const names = new Set(files.map((f) => f.replace(/\.md$/, '')));
  await rm(out, { recursive: true, force: true });
  await mkdir(out, { recursive: true });
  for (const file of files) {
    const name = file.replace(/\.md$/, '');
    const text = await readFile(join(docs, file), 'utf8');
    await writeFile(join(out, file), convert(text, name, names));
  }
  await rm(images, { recursive: true, force: true });
  await mkdir(images, { recursive: true });
  const shots = join(docs, 'screenshots');
  if (existsSync(shots)) {
    for (const file of await readdir(shots)) await copyFile(join(shots, file), join(images, file));
  }
  console.log(`sync-docs: ${files.length} pages, images in public/docs-img`);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main().catch((err) => {
    console.error(err);
    process.exit(1);
  });
}
