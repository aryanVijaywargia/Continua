#!/usr/bin/env node
// Generates web/src/data/repo-stats.json from local `git log`.
// Bundled at build time so the landing page heatmap reflects real commit
// activity without depending on the GitHub API (the repo is private).
//
// Output shape mirrors a slimmed-down version of GitHub's /stats/commit_activity:
//   {
//     "generatedAt": ISO8601,
//     "branch": string,
//     "commitTotal": number,
//     "firstCommitAt": ISO8601 | null,
//     "weeks": [
//       { "weekStart": "YYYY-MM-DD", "total": N, "days": [d0..d6] }   // Sun..Sat, UTC
//     ]
//   }

import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(__dirname, '..');
const outPath = resolve(webRoot, 'src/data/repo-stats.json');

function tryGit(args) {
  try {
    return execFileSync('git', args, { cwd: webRoot, encoding: 'utf8' }).trim();
  } catch {
    return null;
  }
}

function startOfUtcWeek(epochSec) {
  // Snap to the most recent Sunday at 00:00 UTC, matching GitHub's week buckets.
  const d = new Date(epochSec * 1000);
  const day = d.getUTCDay();
  d.setUTCHours(0, 0, 0, 0);
  d.setUTCDate(d.getUTCDate() - day);
  return Math.floor(d.getTime() / 1000);
}

function isoDate(epochSec) {
  return new Date(epochSec * 1000).toISOString().slice(0, 10);
}

function writeFallback(reason) {
  const empty = {
    generatedAt: new Date().toISOString(),
    branch: null,
    commitTotal: 0,
    firstCommitAt: null,
    weeks: [],
    note: `no git data available (${reason})`,
  };
  mkdirSync(dirname(outPath), { recursive: true });
  writeFileSync(outPath, JSON.stringify(empty, null, 2) + '\n');
  console.warn(`[gen-repo-stats] git unavailable, wrote empty stats: ${reason}`);
}

if (!tryGit(['rev-parse', '--show-toplevel'])) {
  writeFallback('not a git checkout');
  process.exit(0);
}

const branch = tryGit(['rev-parse', '--abbrev-ref', 'HEAD']) || 'HEAD';
const log = tryGit(['log', '--no-merges', '--pretty=format:%at']);
if (!log) {
  writeFallback('git log returned no commits');
  process.exit(0);
}

const timestamps = log
  .split('\n')
  .map((line) => Number.parseInt(line, 10))
  .filter((n) => Number.isFinite(n))
  .sort((a, b) => a - b);

if (timestamps.length === 0) {
  writeFallback('empty commit history');
  process.exit(0);
}

const firstCommitAt = timestamps[0];
const firstWeek = startOfUtcWeek(firstCommitAt);
const lastWeek = startOfUtcWeek(Math.floor(Date.now() / 1000));

const weekMap = new Map();
for (let w = firstWeek; w <= lastWeek; w += 7 * 24 * 60 * 60) {
  weekMap.set(w, { weekStart: isoDate(w), total: 0, days: [0, 0, 0, 0, 0, 0, 0] });
}

for (const ts of timestamps) {
  const w = startOfUtcWeek(ts);
  const bucket = weekMap.get(w);
  if (!bucket) continue;
  const day = new Date(ts * 1000).getUTCDay();
  bucket.days[day] += 1;
  bucket.total += 1;
}

const weeks = Array.from(weekMap.values());

const payload = {
  generatedAt: new Date().toISOString(),
  branch,
  commitTotal: timestamps.length,
  firstCommitAt: new Date(firstCommitAt * 1000).toISOString(),
  weeks,
};

if (!existsSync(dirname(outPath))) {
  mkdirSync(dirname(outPath), { recursive: true });
}
writeFileSync(outPath, JSON.stringify(payload, null, 2) + '\n');
console.log(
  `[gen-repo-stats] wrote ${weeks.length} weeks · ${payload.commitTotal} commits · branch=${branch}`
);
