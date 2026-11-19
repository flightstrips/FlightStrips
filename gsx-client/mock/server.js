// A stand-in for GET /api/gsx/stand, so the handler script can be exercised
// without a controller on the other end. Node, no dependencies:
//
//     node gsx-client/mock/server.js
//     ngrok http 8787
//
// then point API_BASE in the deployed handler at the ngrok URL. Edit
// assignment.json while the sim runs to play the controller: the file is read
// on every request, so a change lands on the handler's next poll, within five
// seconds.
//
// It answers the real contract and nothing more - same three fields, same
// shape - so anything that works here works against FlightStrips.

const http = require("http");
const fs = require("fs");
const path = require("path");

const PORT = Number(process.env.PORT || 8787);
const FILE = path.join(__dirname, "assignment.json");

function log(message) {
  console.log(`${new Date().toISOString().slice(11, 19)}  ${message}`);
}

const server = http.createServer((req, res) => {
  // Split the raw target rather than going through new URL(). An API_BASE with
  // a trailing slash asks for "//api/gsx/stand", and new URL() reads a leading
  // "//" as protocol-relative - so it would take "api" for the host and never
  // see the path at all. Answering that with a 404 cost a whole test flight:
  // the handler treats a failed fetch as transient and just keeps polling, so
  // nothing about it is visible from the cockpit.
  const [rawPath, rawQuery] = req.url.split("?");
  const pathname = rawPath.replace(/\/{2,}/g, "/");
  const query = new URLSearchParams(rawQuery || "");
  if (pathname !== "/api/gsx/stand") {
    res.writeHead(404, { "Content-Type": "application/json" });
    res.end('{"error":"try /api/gsx/stand"}');
    return;
  }

  const callsign = query.get("callsign") || "?";
  const icao = query.get("icao") || "?";
  const scenery = query.get("scenery") || "?";

  let payload;
  try {
    payload = JSON.parse(fs.readFileSync(FILE, "utf8"));
  } catch (err) {
    // Answer 503 rather than 500: that is what the handler is built to ride
    // out, so a half-saved edit costs one skipped poll instead of a stall.
    log(`${callsign} -> 503, assignment.json is not valid JSON (${err.message})`);
    res.writeHead(503, { "Content-Type": "application/json" });
    res.end('{"error":"assignment.json is not valid JSON"}');
    return;
  }

  const body = JSON.stringify({
    stand: payload.stand ?? null,
    pushback: payload.pushback ?? null,
    revision: payload.revision ?? "mock",
  });

  // Deliberately no ETag. The real endpoint sends one and the handler polls
  // with If-None-Match, which is right in production and a nuisance here: a
  // 304 would hide an edit to the file until something invalidated the tag.
  res.writeHead(200, {
    "Content-Type": "application/json",
    "Cache-Control": "no-store",
  });
  res.end(body);

  log(`${callsign} at ${icao} (${scenery}) -> ${body}`);
});

server.listen(PORT, () => {
  log(`serving ${path.basename(FILE)} on http://localhost:${PORT}/api/gsx/stand`);
  log("edit assignment.json to change what the pilot is told");
});
