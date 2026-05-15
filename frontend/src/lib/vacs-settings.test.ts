import { beforeEach, describe, expect, it } from "vitest";
import {
  buildVacsWsUrl,
  getVacsHost,
  normalizeVacsHostInput,
  setVacsHost,
} from "./vacs-settings";

describe("vacs-settings", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  describe("normalizeVacsHostInput", () => {
    it("returns empty for blank input", () => {
      expect(normalizeVacsHostInput("  ")).toBe("");
    });

    it("strips ws URL prefix, port, and path", () => {
      expect(normalizeVacsHostInput("ws://192.168.0.5:9600/ws")).toBe("192.168.0.5");
      expect(normalizeVacsHostInput("wss://vacs-pc.local:9600/ws")).toBe("vacs-pc.local");
    });

    it("keeps plain hostname or IPv4", () => {
      expect(normalizeVacsHostInput("10.0.0.2")).toBe("10.0.0.2");
      expect(normalizeVacsHostInput("my-pc")).toBe("my-pc");
    });
  });

  describe("buildVacsWsUrl", () => {
    it("uses localhost when host is empty", () => {
      expect(buildVacsWsUrl("")).toBe("ws://localhost:9600/ws");
      expect(buildVacsWsUrl()).toBe("ws://localhost:9600/ws");
    });

    it("uses provided host", () => {
      expect(buildVacsWsUrl("192.168.1.5")).toBe("ws://192.168.1.5:9600/ws");
    });

    it("reads persisted host when argument omitted", () => {
      setVacsHost("10.0.0.8");
      expect(buildVacsWsUrl()).toBe("ws://10.0.0.8:9600/ws");
      expect(getVacsHost()).toBe("10.0.0.8");
    });
  });
});
