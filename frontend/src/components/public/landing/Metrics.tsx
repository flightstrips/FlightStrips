import { useEffect, useRef, useState, type RefObject } from "react";
import { METRICS } from "./content";

const DURATION_MS = 900;

function prefersReducedMotion(): boolean {
  return typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/**
 * Counts once, on first entry, and only when motion is welcome. Under
 * `prefers-reduced-motion` the final value is rendered immediately and no
 * observer is created, so the number is never conveyed by animation alone.
 *
 * Returns a tuple rather than an object: bundling the ref alongside the value
 * reads as ref access during render to the react-hooks lint rule.
 */
function useCountUp(target: number): [RefObject<HTMLSpanElement | null>, number] {
  const ref = useRef<HTMLSpanElement>(null);
  // Seeded at render time rather than reset inside the effect, which would
  // cascade an extra render on mount.
  const [displayed, setDisplayed] = useState(() => (prefersReducedMotion() ? target : 0));

  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    if (prefersReducedMotion()) return;

    let frame = 0;

    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return;
        observer.disconnect();

        const start = performance.now();
        const tick = (now: number) => {
          const progress = Math.min((now - start) / DURATION_MS, 1);
          // Ease-out cubic: quick off the mark, settling onto the value.
          const eased = 1 - Math.pow(1 - progress, 3);
          setDisplayed(Math.round(target * eased));
          if (progress < 1) frame = requestAnimationFrame(tick);
        };
        frame = requestAnimationFrame(tick);
      },
      { threshold: 0.4 },
    );

    observer.observe(node);
    return () => {
      observer.disconnect();
      cancelAnimationFrame(frame);
    };
  }, [target]);

  return [ref, displayed];
}

function Metric({ target, label, detail }: { target: number; label: string; detail: string }) {
  const [ref, displayed] = useCountUp(target);

  return (
    <div className="border-t border-[var(--fsl-line)] pt-6">
      {/* The accessible name is the real figure, whatever the animation is mid-count. */}
      <span ref={ref} className="fsl-metric__value mb-4" aria-label={String(target)}>
        <span aria-hidden="true">{displayed}</span>
      </span>
      <p className="mb-2 text-[17px] font-medium">{label}</p>
      <p className="max-w-[28ch] text-[14px] leading-relaxed text-[var(--fsl-ink-muted)]">{detail}</p>
    </div>
  );
}

export function Metrics() {
  return (
    <div className="grid gap-x-10 gap-y-12 sm:grid-cols-2 lg:grid-cols-4">
      {METRICS.map((metric) => (
        <Metric key={metric.label} target={metric.value} label={metric.label} detail={metric.detail} />
      ))}
    </div>
  );
}
