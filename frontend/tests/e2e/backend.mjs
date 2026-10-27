import {spawn, spawnSync} from "node:child_process";
import {createWriteStream, mkdirSync} from "node:fs";
import {resolve} from "node:path";

mkdirSync("aman-e2e-artifacts", {recursive: true});
const log = createWriteStream("aman-e2e-artifacts/backend.log");
const binary = resolve(`aman-e2e-artifacts/backend${process.platform === "win32" ? ".exe" : ""}`);
const build = spawnSync("go", ["build", "-o", binary, "./internal/testing/amanbrowser"], {cwd: "../backend", encoding: "utf8"});
if (build.status !== 0) {process.stderr.write(build.stderr); process.exit(1);}
const child = spawn(binary, [], {cwd: "../backend", windowsHide: true});
for (const output of [child.stdout, child.stderr]) output.on("data", data => log.write(data));
child.on("error", error => {process.stderr.write(String(error)); process.exitCode = 1;});
child.on("exit", code => {log.end(); process.exit(code ?? 1);});
for (const signal of ["SIGINT", "SIGTERM"]) process.on(signal, async () => {
  try {await fetch("http://127.0.0.1:4180/e2e/shutdown", {method: "POST"});}
  catch {child.kill(signal);}
});
