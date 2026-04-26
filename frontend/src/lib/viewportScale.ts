const BASELINE_WIDTH = 1920;
const BASELINE_HEIGHT = 1080;

export const toVw = (px: number) => `${(px / BASELINE_WIDTH) * 100}vw`;
export const toDvh = (px: number) => `${(px / BASELINE_HEIGHT) * 100}dvh`;
export const scalePx = (px: number) => `min(${toVw(px)}, ${toDvh(px)})`;
