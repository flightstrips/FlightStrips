export interface ClickPoint {
  label: string;
  /** Percentage from left edge of the map container */
  left: string;
  /** Percentage from top edge of the map container */
  top: string;
}

/**
 * EKCH pushback release points positioned over apron_push.png.
 * Positions are percentages relative to the image content area (1920×768).
 * Derived from SVG layout: (cx - 140) / 1920 and (cy - 140) / 768.
 * One unknown label is marked TODO — identify from the map and replace.
 */
export const RELEASE_POINTS: ClickPoint[] = [
  { label: "K1",  left: "2.89%",  top: "39.71%" },
  { label: "Z1",  left: "7.19%",  top: "35.29%" },
  { label: "Z2",  left: "9.77%",  top: "39.19%" },
  { label: "J1",  left: "12.52%", top: "33.85%" },
  { label: "Z3",  left: "13.00%", top: "43.62%" },
  { label: "J2",  left: "13.88%", top: "28.58%" },
  { label: "Z4",  left: "16.07%", top: "46.98%" },
  { label: "J3",  left: "20.08%", top: "24.93%" },
  { label: "Z5",  left: "22.11%", top: "46.09%" },
  { label: "A50", left: "24.87%", top: "32.68%" },
  { label: "J4",  left: "25.55%", top: "26.04%" },
  { label: "L3",  left: "26.30%", top: "39.32%" },
  { label: "L2",  left: "28.70%", top: "34.11%" },
  { label: "Y0",  left: "29.48%", top: "44.92%" },
  { label: "Z6",  left: "31.56%", top: "56.90%" },
  { label: "Y1",  left: "32.99%", top: "49.87%" },
  { label: "Z7",  left: "36.98%", top: "64.06%" },
  { label: "M1",  left: "37.68%", top: "44.14%" },
  { label: "Y2",  left: "38.41%", top: "56.90%" },
  { label: "Z8", left: "40.42%", top: "69.66%" },
  { label: "Y3",  left: "43.30%", top: "64.97%" },
  { label: "P2",  left: "46.54%", top: "57.68%" },
  { label: "P1",  left: "46.54%", top: "46.09%" },
  { label: "Q1",  left: "49.58%", top: "47.92%" },
  { label: "Q2",  left: "50.29%", top: "63.15%" },
  { label: "Y4",  left: "55.68%", top: "82.16%" },
  { label: "Z9",  left: "55.68%", top: "91.80%" },
  { label: "R1",  left: "60.31%", top: "47.92%" },
  { label: "R2",  left: "60.68%", top: "60.81%" },
  { label: "R3",  left: "61.41%", top: "72.79%" },
  { label: "R3",  left: "61.41%", top: "78.26%" },
  { label: "S1",  left: "63.88%", top: "47.92%" },
  { label: "S2",  left: "64.48%", top: "60.81%" },
  { label: "S3",  left: "64.94%", top: "72.79%" },
  { label: "S4",  left: "65.23%", top: "78.26%" },
  { label: "V3",   left: "64.17%", top: "84.25%" },
  { label: "W2",  left: "68.59%", top: "66.06%" },
  { label: "V4",  left: "70.65%", top: "83.14%" },
  { label: "U1",  left: "77.40%", top: "43.36%" },
  { label: "W3",  left: "77.71%", top: "63.93%" },
  { label: "F89",  left: "80.42%", top: "70.59%" },
  { label: "U2",  left: "81.51%", top: "34.70%" },
  { label: "T1",  left: "85.49%", top: "49.09%" },
  { label: "T2",  left: "86.20%", top: "37.76%" },
  { label: "T3",  left: "89.03%", top: "29.04%" },
  { label: "V5", left: "90.94%", top: "50.85%" },
  { label: "T4",  left: "92.32%", top: "18.23%" },
  { label: "T5",  left: "95.14%", top: "9.70%"  },
];
