import {spawnSync} from "node:child_process";
import {existsSync, mkdirSync} from "node:fs";

mkdirSync("e2e-artifacts", {recursive: true});
function run(command, args) {
  const result = spawnSync(command, args, {stdio: "inherit", windowsHide: true});
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
if (!process.argv.includes("--project=strips")) {
  if (process.env.E2E_NATIVE_CACHE_HIT === "true") {
    const decoder = `e2e-artifacts/native/aman-live-decoder${process.platform === "win32" ? ".exe" : ""}`;
    if (!existsSync(decoder)) throw new Error(`native cache hit but decoder is missing: ${decoder}`);
    console.log("Using native decoder from an exact source/toolchain cache match; live-frame assertions still run.");
  } else {
    run("cmake", ["-S", "tests/e2e/native", "-B", "e2e-artifacts/native", "-G", "Ninja", "-DCMAKE_BUILD_TYPE=Release"]);
    run("cmake", ["--build", "e2e-artifacts/native", "--target", "aman-live-decoder", "--parallel", "2"]);
  }
}
run(process.execPath, ["node_modules/@playwright/test/cli.js", "test", "-c", "playwright.e2e.config.ts", ...process.argv.slice(2)]);
