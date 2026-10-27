import {spawn, spawnSync} from "node:child_process";
import {createWriteStream, mkdirSync} from "node:fs";

mkdirSync("aman-e2e-artifacts", {recursive: true});
// Serve the normal production build, including the actual router and providers.
for (const args of [["node_modules/typescript/bin/tsc", "-b"], ["node_modules/vite/bin/vite.js", "build"]]) {
  const build = spawnSync(process.execPath, args, {encoding: "utf8", maxBuffer: 8 * 1024 * 1024});
  if (build.status !== 0) {process.stderr.write(build.stdout + build.stderr); process.exit(1);}
}
const log = createWriteStream("aman-e2e-artifacts/frontend.log");
const child = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "preview", "--host", "127.0.0.1", "--port", "4181", "--strictPort"], {windowsHide: true});
for (const output of [child.stdout, child.stderr]) output.on("data", data => log.write(data));
child.on("error", error => {process.stderr.write(String(error)); process.exitCode = 1;});
child.on("exit", code => {log.end(); process.exit(code ?? 1);});
for (const signal of ["SIGINT", "SIGTERM"]) process.on(signal, () => child.kill(signal));
