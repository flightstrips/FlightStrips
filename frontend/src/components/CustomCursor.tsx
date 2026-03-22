import { useEffect, useRef } from "react";

function getElementBgColor(el: Element | null): [number, number, number] | null {
  while (el && el !== document.body) {
    const bg = window.getComputedStyle(el).backgroundColor;
    const match = bg.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/);
    if (match) {
      if (bg.startsWith("rgba")) {
        const aMatch = bg.match(/rgba\(\d+,\s*\d+,\s*\d+,\s*([\d.]+)/);
        if (aMatch && parseFloat(aMatch[1]) < 0.1) {
          el = el.parentElement;
          continue;
        }
      }
      return [parseInt(match[1]), parseInt(match[2]), parseInt(match[3])];
    }
    el = el.parentElement;
  }
  return null;
}

function luminance(r: number, g: number, b: number) {
  return 0.299 * r + 0.587 * g + 0.114 * b;
}

const CURSOR_LUM = luminance(136, 136, 136); // grey #888

function pickCursorColor(bg: [number, number, number] | null): string {
  if (!bg) return "#888";
  const [r, g, b] = bg;
  const lum = luminance(r, g, b);
  const chroma = Math.max(r, g, b) - Math.min(r, g, b);

  if ((chroma < 40 && Math.abs(lum - CURSOR_LUM) < 60) || Math.abs(lum - CURSOR_LUM) < 50) {
    return lum > 127 ? "#222" : "#eee";
  }
  return "#888";
}

// Total pixel size of the cursor (full width and height of the cross)
const CURSOR_SIZE = 20;
const HALF = CURSOR_SIZE / 2;

const base: React.CSSProperties = {
  position: "fixed",
  top: 0,
  left: 0,
  background: "#888",
  pointerEvents: "none",
  zIndex: 99999,
  willChange: "transform",
};

export function CustomCursor() {
  const hlRef = useRef<HTMLDivElement>(null);
  const hrRef = useRef<HTMLDivElement>(null);
  const vtRef = useRef<HTMLDivElement>(null);
  const vbRef = useRef<HTMLDivElement>(null);
  const colorFrameRef = useRef<number | null>(null);
  const lastPos = useRef({ x: -100, y: -100 });

  useEffect(() => {
    const applyColor = (color: string) => {
      for (const el of [hlRef.current, hrRef.current, vtRef.current, vbRef.current]) {
        if (el) el.style.background = color;
      }
    };

    const onMove = (e: MouseEvent) => {
      const x = e.clientX;
      const y = e.clientY;
      lastPos.current = { x, y };

      if (hlRef.current) hlRef.current.style.transform = `translate(${x - HALF}px,${y - 1}px)`;
      if (hrRef.current) hrRef.current.style.transform = `translate(${x}px,${y - 1}px)`;
      if (vtRef.current) vtRef.current.style.transform = `translate(${x - 1}px,${y - HALF}px)`;
      if (vbRef.current) vbRef.current.style.transform = `translate(${x - 1}px,${y}px)`;

      if (colorFrameRef.current !== null) return;
      colorFrameRef.current = requestAnimationFrame(() => {
        colorFrameRef.current = null;
        const { x: cx, y: cy } = lastPos.current;
        const el = document.elementFromPoint(cx, cy);
        applyColor(pickCursorColor(getElementBgColor(el)));
      });
    };

    window.addEventListener("mousemove", onMove, { passive: true });
    return () => {
      window.removeEventListener("mousemove", onMove);
      if (colorFrameRef.current !== null) cancelAnimationFrame(colorFrameRef.current);
    };
  }, [hlRef, hrRef, vtRef, vbRef]);

  return (
    <>
      <style>{`* { cursor: none !important; }`}</style>
      <div ref={hlRef} style={{ ...base, width: HALF, height: 2 }} />
      <div ref={hrRef} style={{ ...base, width: HALF, height: 2 }} />
      <div ref={vtRef} style={{ ...base, width: 2, height: HALF }} />
      <div ref={vbRef} style={{ ...base, width: 2, height: HALF }} />
    </>
  );
}
