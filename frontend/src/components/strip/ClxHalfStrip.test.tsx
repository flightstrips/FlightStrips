import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Bay } from "@/api/models";
import { ClxHalfStrip } from "./ClxHalfStrip";
import { HALF_STRIP_VARIANT_BG, LIGHT_GRAY_STRIP_FRAME } from "./halfStripFrame";
import { STRIP_FRAME_COLOR } from "./shared";

vi.mock("./shared", async importOriginal => {
  const actual = await importOriginal<typeof import("./shared")>();
  return {
    ...actual,
    useStripBg: (_runway: string | undefined, normalBg: string) => ({ bg: normalBg, textWhite: false }),
  };
});

describe("CLR DEL half-strip background", () => {
  it.each([Bay.Push, Bay.Taxi])("uses gray in bay %s", bay => {
    const { container } = render(<ClxHalfStrip callsign="SAS123" bay={bay} />);

    expect(container.firstElementChild?.firstElementChild).toHaveStyle({
      backgroundColor: HALF_STRIP_VARIANT_BG["APN-PUSH"],
    });
    expect(container.firstElementChild).toHaveStyle({
      backgroundColor: LIGHT_GRAY_STRIP_FRAME,
    });
    expect(container.firstElementChild?.firstElementChild?.firstElementChild).toHaveStyle({
      borderRight: `2px solid ${LIGHT_GRAY_STRIP_FRAME}`,
    });
  });

  it("keeps the normal departure background in other bays", () => {
    const { container } = render(<ClxHalfStrip callsign="SAS123" bay={Bay.Cleared} />);

    expect(container.firstElementChild?.firstElementChild).toHaveStyle({
      backgroundColor: "var(--color-strip-dep-bg)",
    });
    expect(container.firstElementChild).toHaveStyle({
      backgroundColor: STRIP_FRAME_COLOR,
    });
  });
});
