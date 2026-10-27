// Reproduce the original visible failure without touching the running outage
// fix checkout: reject a complete valid replacement when one flight becomes
// prediction_unreliable. The real engine must generate the warning itself.
import {spawnSync} from "node:child_process";
import {copyFileSync, existsSync, mkdirSync, readFileSync, writeFileSync} from "node:fs";

mkdirSync("aman-e2e-artifacts/mutation", {recursive: true});
const path = "src/api/aman.ts";
const original = readFileSync(path);
const needle = "export function replaceAMANState(current: AMANState | null, input: unknown): AMANReplacementResult {";
const mutation = `${needle}\n  if (isAMANStateEvent(input) && input.data.warnings?.some(warning => warning.code === "prediction_unreliable")) {\n    return {state: null, status: "degraded", error: "invalid_aman_state", accepted: false};\n  }`;
if (!original.toString().includes(needle)) throw new Error("mutation target changed; update the regression proof");
let result;
try {
  writeFileSync(path, original.toString().replace(needle, mutation));
  result = spawnSync(process.execPath, ["node_modules/@playwright/test/cli.js", "test", "-c", "playwright.aman.config.ts", "--reporter=json", "--output=aman-e2e-artifacts/mutation/results"], {encoding: "utf8", maxBuffer: 32 * 1024 * 1024});
} finally {
  writeFileSync(path, original);
}
writeFileSync("aman-e2e-artifacts/mutation/report.json", result.stdout ?? "");
writeFileSync("aman-e2e-artifacts/mutation/stderr.log", result.stderr ?? "");
for (const name of ["backend.log", "frontend.log"]) {
  if (existsSync(`aman-e2e-artifacts/${name}`)) copyFileSync(`aman-e2e-artifacts/${name}`, `aman-e2e-artifacts/mutation/${name}`);
}
if (result.error) throw result.error;
const report = JSON.parse(result.stdout);
const failures = [];
function visit(suite) {
  for (const spec of suite.specs ?? []) for (const test of spec.tests) for (const run of test.results) {
    if (run.status === "failed") failures.push(...run.errors);
  }
  for (const child of suite.suites ?? []) visit(child);
}
for (const suite of report.suites ?? []) visit(suite);
// A build, startup, native-tool or unrelated UI failure is not a mutation kill.
const caught = result.status === 1 && failures.some(error =>
  /Select SAS101/.test(error.message ?? "") && /toBeVisible/.test(error.message ?? ""));
if (!caught) {
  process.stderr.write(result.stdout ?? "");
  process.stderr.write(result.stderr ?? "");
  throw new Error("E2E did not catch the original lost-healthy-target regression at its visible-target assertion");
}
console.log("Mutation killed: a real unreliable prediction caused valid SAS101 targets to disappear. Correct consumer restored byte-for-byte.");
