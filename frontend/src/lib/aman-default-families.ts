const SWEDISH_FAMILIES = new Set(["TIDVU", "ERNOV"]);
const ALL_EKDK_CENTRE = new Set(["EKDK_UC_CTR", "EKDK_UV_CTR", "EKDK_UN_CTR", "EKDK_CTR"]);
const ALL_EKCH_APPROACH = new Set(["EKCH_P_APP", "EKCH_P1_APP", "EKCH_P2_APP"]);
const TIDVU_OWNERS = ["ESMS_APP", "ESMM_7_CTR", "ESMM_8_CTR", "ESMM_Y_CTR"] as const;
const ERNOV_OWNERS = ["ESMM_5_CTR", "ESMM_K_CTR"] as const;

export interface AMANOnlineController {
  callsign: string;
  observer?: boolean;
}

/**
 * Default STAR families to emphasise when ACC is opened. An empty result means
 * no restriction (everything shown).
 */
export function defaultACCFamilies(
  position: string,
  controllers: readonly AMANOnlineController[],
  availableFamilies: ReadonlySet<string>,
): string[] {
  const own = position.trim().toUpperCase();
  const online = new Set(controllers.filter((controller) => controller.observer !== true).map((controller) => controller.callsign.trim().toUpperCase()));
  online.add(own);
  const offline = (callsign: string) => !online.has(callsign);
  const anyOnline = (callsigns: readonly string[]) => callsigns.some((callsign) => online.has(callsign));
  const selected = new Set<string>();

  switch (own) {
    case "EKDK_B_CTR":
      selected.add("MONAK");
      if (offline("EKDK_D_CTR") || offline("EKDK_E_CTR") || offline("EKDK_I_CTR")) {
        selected.add("TUDLO");
        selected.add("TESPI");
      }
      break;
    case "EKDK_I_CTR":
      if (offline("EKDK_B_CTR")) selected.add("MONAK");
      if (offline("EKDK_D_CTR")) selected.add("TUDLO");
      if (offline("EKDK_E_CTR")) selected.add("TESPI");
      break;
    case "EKDK_D_CTR":
      selected.add("TUDLO");
      if (offline("EKDK_E_CTR")) selected.add("TESPI");
      if (offline("EKDK_B_CTR") || offline("EKDK_I_CTR")) selected.add("MONAK");
      break;
    case "EKDK_E_CTR":
      selected.add("TESPI");
      if (offline("EKDK_D_CTR")) selected.add("TUDLO");
      if (offline("EKDK_I_CTR") && offline("EKDK_B_CTR") && offline("EKDK_D_CTR")) selected.add("MONAK");
      break;
    case "ESMS_APP":
    case "ESMM_7_CTR":
    case "ESMM_8_CTR":
    case "ESMM_Y_CTR":
      selected.add("TIDVU");
      break;
    case "ESMM_5_CTR":
      selected.add("ERNOV");
      break;
    case "ESMM_K_CTR":
      selected.add("ERNOV");
      if (!anyOnline(TIDVU_OWNERS)) selected.add("TIDVU");
      break;
    case "ESMM_2_CTR":
      if (!anyOnline(TIDVU_OWNERS)) selected.add("TIDVU");
      if (!anyOnline(ERNOV_OWNERS)) selected.add("ERNOV");
      break;
    case "EKCH_W_APP":
      ["TESPI", "TUDLO", "ERNOV"].forEach((family) => selected.add(family));
      break;
    case "EKCH_O_APP":
      ["TIDVU", "MONAK"].forEach((family) => selected.add(family));
      break;
    default:
      if (ALL_EKDK_CENTRE.has(own)) {
        availableFamilies.forEach((family) => {
          if (!SWEDISH_FAMILIES.has(family)) selected.add(family);
        });
      } else if (ALL_EKCH_APPROACH.has(own)) {
        return [];
      }
  }

  return [...selected].filter((family) => availableFamilies.has(family));
}
