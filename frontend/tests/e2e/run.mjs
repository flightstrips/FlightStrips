import {spawnSync} from "node:child_process";
import {mkdirSync} from "node:fs";

mkdirSync("e2e-artifacts", {recursive: true});
function run(command, args) {
  const result = spawnSync(command, args, {stdio: "inherit", windowsHide: true});
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
if (!process.argv.includes("--project=strips")) {
  run("cmake", ["-S", "tests/e2e/native", "-B", "e2e-artifacts/native", "-G", "Ninja", "-DCMAKE_BUILD_TYPE=Release"]);
  run("cmake", ["--build", "e2e-artifacts/native", "--target", "aman-live-decoder", "--parallel", "2"]);
}
run(process.execPath, ["node_modules/@playwright/test/cli.js", "test", "-c", "playwright.e2e.config.ts", ...process.argv.slice(2)]);
