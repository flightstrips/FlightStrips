import { beforeEach, expect, it } from "vitest";
import { isVacsIntegrationEnabled, setVacsIntegrationEnabled } from "./vacs-settings";
beforeEach(() => localStorage.clear());
it("keeps controls opt-in independently of legacy host settings", () => {
  localStorage.setItem("flightstrips.vacs.host", "192.168.1.5");
  expect(isVacsIntegrationEnabled()).toBe(false);
  setVacsIntegrationEnabled(true); expect(isVacsIntegrationEnabled()).toBe(true);
  setVacsIntegrationEnabled(false); expect(isVacsIntegrationEnabled()).toBe(false);
});
