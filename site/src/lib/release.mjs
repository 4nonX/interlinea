// The latest release, read once per build from the GitHub API. The site is
// rebuilt when a release is published, so the download section always names
// the files of the newest one. Without network access the build falls back
// to the newest version in CHANGELOG.md and the file names every release
// carries, without their sizes. RELEASE_JSON names a file with the API's
// answer instead, for building without access to the API.
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

export const repo = '4nonX/interlinea';
export const github = `https://github.com/${repo}`;

function fromChangelog() {
  const text = readFileSync(resolve(process.cwd(), '..', 'CHANGELOG.md'), 'utf8');
  const m = text.match(/^## \[(\d+\.\d+\.\d+)\] - (\d{4}-\d{2}-\d{2})/m);
  if (!m) throw new Error('release: no released version in CHANGELOG.md');
  const v = m[1];
  const names = [
    `Interlinea-${v}-windows-x64-setup.exe`,
    `Interlinea-${v}-windows-x64-portable.zip`,
    `Interlinea-${v}-macos-universal.dmg`,
    `Interlinea-${v}-linux-x86_64.AppImage`,
    `interlinea-server-${v}-linux-amd64.tar.gz`,
    `interlinea-server-${v}-linux-arm64.tar.gz`,
    `interlinea-server-${v}-macos-amd64.tar.gz`,
    `interlinea-server-${v}-macos-arm64.tar.gz`,
    `interlinea-server-${v}-windows-amd64.zip`,
    `interlinea-web-${v}.zip`,
    'SHA256SUMS',
  ];
  return {
    version: v,
    date: m[2],
    assets: names.map((name) => ({ name, size: 0, url: `${github}/releases/download/v${v}/${name}` })),
  };
}

let cached;

export async function latestRelease() {
  if (cached) return cached;
  const headers = { accept: 'application/vnd.github+json', 'user-agent': 'interlinea-site' };
  if (process.env.GITHUB_TOKEN) headers.authorization = `Bearer ${process.env.GITHUB_TOKEN}`;
  if (process.env.RELEASE_JSON) {
    const j = JSON.parse(readFileSync(process.env.RELEASE_JSON, 'utf8'));
    cached = {
      version: j.tag_name.replace(/^v/, ''),
      date: j.published_at.slice(0, 10),
      assets: j.assets.map((a) => ({ name: a.name, size: a.size, url: a.browser_download_url })),
    };
    return cached;
  }
  try {
    const url = `https://api.github.com/repos/${repo}/releases/latest`;
    let res = await fetch(url, { headers });
    if (res.status === 401 && headers.authorization) {
      delete headers.authorization;
      res = await fetch(url, { headers });
    }
    if (!res.ok) throw new Error(`GitHub answered ${res.status}`);
    const j = await res.json();
    cached = {
      version: j.tag_name.replace(/^v/, ''),
      date: j.published_at.slice(0, 10),
      assets: j.assets.map((a) => ({ name: a.name, size: a.size, url: a.browser_download_url })),
    };
  } catch (err) {
    if (process.env.REQUIRE_RELEASE) throw err;
    console.warn(`release: ${err.message}; using CHANGELOG.md`);
    cached = fromChangelog();
  }
  return cached;
}

export function asset(release, pattern) {
  return release.assets.find((a) => pattern.test(a.name));
}

export function size(bytes) {
  if (!bytes) return '';
  return `${Math.round(bytes / 1e6)} MB`;
}

export function longDate(iso) {
  const [y, m, d] = iso.split('-').map(Number);
  const months = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];
  return `${d} ${months[m - 1]} ${y}`;
}
