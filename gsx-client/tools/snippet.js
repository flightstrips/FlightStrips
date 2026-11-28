#!/usr/bin/env node
// Write the ready-to-copy FlightStrips block the profile-maker docs show.
//
//     node gsx-client/tools/snippet.js           # regenerate it
//     node gsx-client/tools/snippet.js --check   # fail if it is out of date
//
// It is the block tools/embed.js adds, with the production FS_API_BASE. It
// runs on its own as a whole handler script, or pasted at the end of one a
// profile already ships. Run this after changing the handler: the
// docs are built from docs/ alone, so they hold a copy, not a reference.

const fs = require("fs");
const path = require("path");
const { DEFAULT_API_BASE, namespacedBlock, wrap } = require("./embed.js");

const OUT = path.join(__dirname, "..", "..", "docs", "src", "snippets", "gsx", "flightstrips_handler.py");

const expected = wrap(namespacedBlock(), JSON.stringify(DEFAULT_API_BASE));

if (process.argv.includes("--check")) {
  // A Windows checkout may hold it with CRLF line endings.
  const current = fs.existsSync(OUT) ? fs.readFileSync(OUT, "utf8").replace(/\r\n/g, "\n") : "";
  if (current !== expected) {
    console.error(`${path.relative(process.cwd(), OUT)} is out of date: run node gsx-client/tools/snippet.js`);
    process.exit(1);
  }
  console.log("snippet is up to date");
} else {
  fs.mkdirSync(path.dirname(OUT), { recursive: true });
  fs.writeFileSync(OUT, expected);
  console.log(`wrote ${path.relative(process.cwd(), OUT)}`);
}
