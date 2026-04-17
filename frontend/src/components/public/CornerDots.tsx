/** Corner markers for industrial grid cells (matches homepage accent) */
export function CornerDots() {
  return (
    <>
      <span className="pointer-events-none absolute top-0 left-0 z-10 h-1.5 w-1.5 -translate-x-px -translate-y-px bg-[#003d48] dark:bg-[#003d48]" />
      <span className="pointer-events-none absolute top-0 right-0 z-10 h-1.5 w-1.5 translate-x-px -translate-y-px bg-[#003d48] dark:bg-[#003d48]" />
      <span className="pointer-events-none absolute bottom-0 left-0 z-10 h-1.5 w-1.5 -translate-x-px translate-y-px bg-[#003d48] dark:bg-[#003d48]" />
      <span className="pointer-events-none absolute bottom-0 right-0 z-10 h-1.5 w-1.5 translate-x-px translate-y-px bg-[#003d48] dark:bg-[#003d48]" />
    </>
  );
}
