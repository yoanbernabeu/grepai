import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

export interface Release {
  version: string;
  date: string;
  added: string[];
  /** Bullets in the other sections (Fixed, Changed, Performance...) */
  otherChanges: number;
}

/**
 * Reads the latest releases from the repository CHANGELOG at build time
 * (the docs are built from ./docs with the full repo checked out).
 * Returns an empty list when the file is missing, so the section hides.
 */
export function latestReleases(count = 3, perRelease = 3): Release[] {
  let text: string;
  try {
    text = readFileSync(resolve(process.cwd(), '../CHANGELOG.md'), 'utf8');
  } catch {
    return [];
  }

  const releases: Release[] = [];
  const blocks = text.split(/^## \[/m).slice(1);
  for (const block of blocks) {
    const header = block.match(/^(\d+\.\d+\.\d+)\] - (\d{4}-\d{2}-\d{2})/);
    if (!header) continue; // skips [Unreleased]

    const sections = block.split(/^### /m).slice(1);
    const addedSection = sections.find((s) => s.startsWith('Added'));
    const otherChanges = sections
      .filter((s) => !s.startsWith('Added'))
      .reduce((total, s) => total + (s.match(/^- /gm)?.length ?? 0), 0);
    const added = addedSection
      ? [...addedSection.matchAll(/^- \*\*([^*]+)\*\*/gm)].map((m) => m[1].trim()).slice(0, perRelease)
      : [];
    if (added.length === 0) continue; // patch releases with fixes only

    releases.push({ version: header[1], date: header[2], added, otherChanges });
    if (releases.length === count) break;
  }
  return releases;
}

/** Latest released version in CHANGELOG.md, e.g. "0.39.0". */
export function latestVersion(): string | undefined {
  try {
    const text = readFileSync(resolve(process.cwd(), '../CHANGELOG.md'), 'utf8');
    return text.match(/^## \[(\d+\.\d+\.\d+)\]/m)?.[1];
  } catch {
    return undefined;
  }
}

export interface RepoStats {
  stars: number | null;
  contributors: number | null;
  releases: number | null;
}

// Anonymous GitHub API calls; on failure the counts are simply not shown.
const githubHeaders: Record<string, string> = { 'User-Agent': 'grepai-docs' };

// Counts from the GitHub API, read from the "last page" link with per_page=1.
async function countPages(url: string): Promise<number | null> {
  const res = await fetch(url, { headers: githubHeaders });
  if (!res.ok) return null;
  const last = res.headers.get('link')?.match(/[?&]page=(\d+)>; rel="last"/);
  if (last) return Number(last[1]);
  const body = await res.json();
  return Array.isArray(body) ? body.length : null;
}

export async function repoStats(): Promise<RepoStats> {
  const base = 'https://api.github.com/repos/yoanbernabeu/grepai';
  try {
    const [repo, contributors, releases] = await Promise.all([
      fetch(base, { headers: githubHeaders }).then((r) => (r.ok ? r.json() : null)),
      countPages(`${base}/contributors?per_page=1`),
      countPages(`${base}/releases?per_page=1`),
    ]);
    return { stars: repo?.stargazers_count ?? null, contributors, releases };
  } catch {
    return { stars: null, contributors: null, releases: null };
  }
}
