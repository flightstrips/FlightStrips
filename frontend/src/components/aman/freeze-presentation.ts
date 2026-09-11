import type {AMANFreezeReason} from "@/api/aman";

export interface AMANFreezePresentation {
  /** Full visible and assistive label; protection never relies on color alone. */
  label: string;
  shortLabel: string;
  tone: string;
}

export function freezePresentation(reason: AMANFreezeReason): AMANFreezePresentation | null {
  switch (reason) {
    case "none":
      return null;
    case "superstable":
      return {label: "Superstable", shortLabel: "SS", tone: "bg-[#dcdcdc] text-[#202020]"};
    case "tma":
      return {label: "TMA entry protection", shortLabel: "TMA", tone: "bg-cyan-950 text-cyan-200"};
    case "manual":
      return {label: "Stable, manual freeze", shortLabel: "S·M", tone: "bg-[#96d796] text-[#202020]"};
  }
}
