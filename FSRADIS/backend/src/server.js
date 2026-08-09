import express from "express";
import cors from "cors";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const app = express();
const port = process.env.PORT || 4000;

app.use(cors());
app.use(express.json());

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const dataPath = path.resolve(__dirname, "../data/mockStrips.json");
const strips = JSON.parse(fs.readFileSync(dataPath, "utf8")).map((strip) => ({
  ...strip,
}));

app.get("/api/health", (_req, res) => {
  res.json({ status: "ok", service: "fsradis-mock-backend" });
});

app.get("/api/strips", (_req, res) => {
  res.json(strips);
});

app.post("/api/strips/:id/status", (req, res) => {
  const { id } = req.params;
  const { status } = req.body;

  if (!status || !["PENDING", "ACTIVE", "COMPLETED"].includes(status)) {
    return res.status(400).json({ message: "Invalid status" });
  }

  const strip = strips.find((item) => item.id === id);
  if (!strip) {
    return res.status(404).json({ message: "Strip not found" });
  }

  strip.status = status;
  strip.updatedAt = new Date().toISOString();
  return res.json(strip);
});

app.listen(port, () => {
  console.log(`FSRADIS mock backend listening on http://localhost:${port}`);
});
