import {spawnSync} from "node:child_process";
import {mkdtempSync, rmSync} from "node:fs";
import {tmpdir} from "node:os";
import {dirname, join, resolve} from "node:path";
import {fileURLToPath} from "node:url";

const frontend = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const tempRoot = resolve(tmpdir());
const outputDir = mkdtempSync(join(tempRoot, "flightstrips-aman-contract-"));
const env = {...process.env, AMAN_CONTRACT_OUTPUT: join(outputDir, "states.json")};
function run(command, args, cwd) {
  const result = spawnSync(command, args, {cwd, env, stdio: "inherit"});
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} failed with exit code ${result.status}`);
}

try {
  run("go", ["test", "./pkg/events/frontend", "-run", "^TestAMANStateConsumerContract$", "-count=1"], resolve(frontend, "../backend"));
  run(process.execPath, [join(frontend, "node_modules/vitest/vitest.mjs"), "run", "--config", "vitest.contract.config.ts"], frontend);
} finally {
  // Only remove the exact disposable directory created above.
  if (dirname(outputDir) === tempRoot && outputDir.startsWith(join(tempRoot, "flightstrips-aman-contract-"))) {
    rmSync(outputDir, {recursive: true, force: true});
  }
}
