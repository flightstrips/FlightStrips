/** Only these established EKCH holdings are displayed, in this order. */
export const EKCH_TMT_HOLDING_ORDER: readonly string[] = ["TIDVU", "OLPIB", "LUGAS", "ROSBI", "ERNOV"];

/** The holding each MAESTRO STAR family feeds. */
export const EKCH_STAR_FAMILY_HOLDING: Readonly<Record<string, string>> = {
  TESPI: "ROSBI",
  TUDLO: "LUGAS",
  MONAK: "OLPIB",
  TIDVU: "TIDVU",
  ERNOV: "ERNOV",
};

/** Holdings for the selected STAR families, in display order. An empty selection shows every holding. */
export function holdingsForFamilies(families: readonly string[]): readonly string[] {
  if (families.length === 0) return EKCH_TMT_HOLDING_ORDER;
  const wanted = new Set(families.map((family) => EKCH_STAR_FAMILY_HOLDING[family]).filter((holding) => holding !== undefined));
  return EKCH_TMT_HOLDING_ORDER.filter((holding) => wanted.has(holding));
}
