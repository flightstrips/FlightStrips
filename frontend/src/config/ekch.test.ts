import { describe, expect, it } from "vitest";
import { HOLDING_POINT_RUNWAYS } from "./ekch";

describe("holding point runway annotations", () => {
  it("provides the release points and TODA distances with their chart positions", () => {
    const runways = Object.fromEntries(
      HOLDING_POINT_RUNWAYS.map(({ runway, points, todaPoints }) => [
        runway,
        {
          points: points.map(({ label, left, top }) => ({ label, left, top })),
          todaPoints: todaPoints.map(({ label, left, top }) => ({ label, left, top })),
        },
      ]),
    );

    expect(runways).toEqual({
      "04L": {
        points: [{ label: "A10", left: "37.7%", top: "40.7%" }],
        todaPoints: [{ label: "3001m", left: "37.7%", top: "78.0%" }],
      },
      "04R": {
        points: [
          { label: "B1", left: "7.5%", top: "42.3%" },
          { label: "B2", left: "16.4%", top: "42.3%" },
          { label: "B3", left: "41.3%", top: "42.3%" },
          { label: "B4", left: "85.5%", top: "42.3%" },
          { label: "C", left: "95.8%", top: "42.3%" },
        ],
        todaPoints: [
          { label: "3302m", left: "8.5%", top: "77.0%" },
          { label: "3203m", left: "16.9%", top: "77.0%" },
          { label: "2797m", left: "42.0%", top: "77.0%" },
          { label: "1941m", left: "95.8%", top: "77.0%" },
        ],
      },
      "12": {
        points: [
          { label: "K1", left: "4.4%", top: "45.1%" },
          { label: "K2", left: "28.8%", top: "45.1%" },
          { label: "K3", left: "40.5%", top: "37.3%" },
          { label: "D", left: "91.9%", top: "37.0%" },
          { label: "F1", left: "5.3%", top: "85.2%" },
          { label: "F2", left: "61.8%", top: "87.9%" },
        ],
        todaPoints: [
          { label: "2800m", left: "26.7%", top: "62.0%" },
          { label: "2699m", left: "35.0%", top: "62.3%" },
          { label: "2481m", left: "47.9%", top: "63.0%" },
          { label: "1798m", left: "94.0%", top: "64.3%" },
        ],
      },
      "22L": {
        points: [
          { label: "V2", left: "53.5%", top: "27.5%" },
          { label: "V1", left: "92.4%", top: "27.5%" },
          { label: "I", left: "41.6%", top: "89.0%" },
        ],
        todaPoints: [
          { label: "2787m", left: "46.6%", top: "63.0%" },
          { label: "3302m", left: "92.4%", top: "63.0%" },
        ],
      },
      "22R": {
        points: [
          { label: "A4", left: "14.2%", top: "30.1%" },
          { label: "A3", left: "30.4%", top: "30.1%" },
          { label: "A2", left: "45.6%", top: "30.1%" },
          { label: "A1", left: "59.6%", top: "30.1%" },
          { label: "E1", left: "51.4%", top: "91.1%" },
        ],
        todaPoints: [
          { label: "3234m", left: "4.5%", top: "73.0%" },
          { label: "3326m", left: "17.1%", top: "73.0%" },
          { label: "3489m", left: "31.2%", top: "73.0%" },
          { label: "3571m", left: "46.2%", top: "73.0%" },
        ],
      },
      "30": {
        points: [
          { label: "G2", left: "16.2%", top: "43.2%" },
          { label: "G1", left: "82.0%", top: "43.2%" },
          { label: "N2", left: "18.4%", top: "88.7%" },
        ],
        todaPoints: [{ label: "2365m", left: "81.8%", top: "67.0%" }],
      },
    });
  });
});
