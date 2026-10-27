import {spawn, spawnSync} from "node:child_process";
import {createWriteStream, mkdirSync} from "node:fs";
import {resolve} from "node:path";

const [suite = "strips", port = "4182"] = process.argv.slice(2);
if (!["aman", "strips"].includes(suite) || !/^\d+$/.test(port)) throw new Error("invalid backend suite or port");
mkdirSync("e2e-artifacts", {recursive: true});
const log = createWriteStream(`e2e-artifacts/backend-${suite}.log`);
const binary = resolve(`e2e-artifacts/backend-${suite}${process.platform === "win32" ? ".exe" : ""}`);
const build = spawnSync("go", ["build", "-o", binary, "./internal/testing/browserharness"], {cwd: "../backend", encoding: "utf8"});
if (build.status !== 0) {process.stderr.write(build.stderr); process.exit(1);}
const child = spawn(binary, ["--suite", suite, "--address", `127.0.0.1:${port}`], {cwd: "../backend", windowsHide: true});
for (const output of [child.stdout, child.stderr]) output.on("data", data => log.write(data));
child.on("error", error => {process.stderr.write(String(error)); process.exitCode = 1;});
child.on("exit", code => {log.end(); process.exit(code ?? 1);});
for (const signal of ["SIGINT", "SIGTERM"]) process.on(signal, async () => {
  try {await fetch(`http://127.0.0.1:${port}/e2e/shutdown`, {method: "POST"});}
  catch {child.kill(signal);}
});
