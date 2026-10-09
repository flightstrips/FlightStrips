const K3 = new Set(["A17", "A4", "A6", "A8", "A18", "A19", "A20", "A21", "A22", "A23"]);
const K2 = new Set(["A25", "A26", "A27", "A28", "A29", "A30", "A31", "A32", "A33", "A34"]);
const NONE = new Set(["RI", "RII", "RIII", "W1"]);
const A_04L = new Set(["A7", "A9", "A11", "A12", "A14", "A15", "B19", "B4", "B6", "B8"]);

/** Default exit taxiway shown on arrival strips, derived from the landing runway and assigned stand. */
export function getDefaultExitTaxiway(runway: string | undefined, stand: string | undefined): string {
  const rwy = (runway ?? "").trim().toUpperCase();
  const st = (stand ?? "").trim().toUpperCase();
  if (!st || (rwy !== "22L" && rwy !== "04L")) return "";
  if (K3.has(st)) return "K3";
  if (K2.has(st)) return "K2";
  if (st.startsWith("G") || NONE.has(st)) return "";
  if (rwy === "04L") return A_04L.has(st) ? "A" : "F";
  return "B";
}
