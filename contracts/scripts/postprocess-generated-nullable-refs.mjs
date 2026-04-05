import { readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repoDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");

function replaceOrThrow(source, pattern, replacement, label) {
  const next =
    pattern instanceof RegExp
      ? source.replace(pattern, replacement)
      : source.replace(pattern, replacement);

  if (next === source) {
    throw new Error(`expected to patch ${label}`);
  }

  return next;
}

function patchTypescript() {
  const targetPath = resolve(repoDir, "generated/typescript/api.ts");
  let source = readFileSync(targetPath, "utf8");

  source = replaceOrThrow(
    source,
    'baseline_span: components["schemas"]["CompareSpanSummary"];',
    'baseline_span: components["schemas"]["CompareSpanSummary"] | null;',
    "typescript baseline_span",
  );
  source = replaceOrThrow(
    source,
    'candidate_span: components["schemas"]["CompareSpanSummary"];',
    'candidate_span: components["schemas"]["CompareSpanSummary"] | null;',
    "typescript candidate_span",
  );
  source = replaceOrThrow(
    source,
    'baseline_event: components["schemas"]["CompareSemanticSummary"];',
    'baseline_event: components["schemas"]["CompareSemanticSummary"] | null;',
    "typescript baseline_event",
  );
  source = replaceOrThrow(
    source,
    'candidate_event: components["schemas"]["CompareSemanticSummary"];',
    'candidate_event: components["schemas"]["CompareSemanticSummary"] | null;',
    "typescript candidate_event",
  );
  source = replaceOrThrow(
    source,
    'current_wait: components["schemas"]["EngineWaitState"];',
    'current_wait: components["schemas"]["EngineWaitState"] | null;',
    "typescript current_wait",
  );
  source = replaceOrThrow(
    source,
    'result: unknown;',
    'result: unknown | null;',
    "typescript engine run result",
  );

  writeFileSync(targetPath, source);
}

function patchGo() {
  const targetPath = resolve(repoDir, "generated/go/server_gen.go");
  let source = readFileSync(targetPath, "utf8");

  source = replaceOrThrow(
    source,
    /(BaselineEvent\s+)(CompareSemanticSummary)(\s+`json:"baseline_event"`)/,
    "$1*$2$3",
    "go baseline_event",
  );
  source = replaceOrThrow(
    source,
    /(CandidateEvent\s+)(CompareSemanticSummary)(\s+`json:"candidate_event"`)/,
    "$1*$2$3",
    "go candidate_event",
  );
  source = replaceOrThrow(
    source,
    /(BaselineSpan\s+)(CompareSpanSummary)(\s+`json:"baseline_span"`)/,
    "$1*$2$3",
    "go baseline_span",
  );
  source = replaceOrThrow(
    source,
    /(CandidateSpan\s+)(CompareSpanSummary)(\s+`json:"candidate_span"`)/,
    "$1*$2$3",
    "go candidate_span",
  );
  source = replaceOrThrow(
    source,
    /(CurrentWait\s+)(EngineWaitState)(\s+`json:"current_wait"`)/,
    "$1*$2$3",
    "go current_wait",
  );

  writeFileSync(targetPath, source);
}

patchTypescript();
patchGo();
