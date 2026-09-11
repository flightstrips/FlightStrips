import { useEffect, useRef, useState } from "react";
import { CAPABILITIES } from "./content";

/**
 * Four capability entries in normal document order, with a sticky index that
 * tracks the one currently in the activation band. The index is decorative:
 * every entry reads correctly with JavaScript off and on small screens, where
 * the sticky column is not rendered at all.
 */
export function CapabilityStory() {
  const [activeIndex, setActiveIndex] = useState(0);
  const entryRefs = useRef<Array<HTMLElement | null>>([]);

  useEffect(() => {
    const nodes = entryRefs.current.filter((node): node is HTMLElement => node !== null);
    if (nodes.length === 0) return;

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue;
          const index = nodes.indexOf(entry.target as HTMLElement);
          if (index >= 0) setActiveIndex(index);
        }
      },
      // Activation band across the middle of the viewport.
      { rootMargin: "-35% 0px -50% 0px", threshold: 0 },
    );

    for (const node of nodes) observer.observe(node);
    return () => observer.disconnect();
  }, []);

  return (
    <div className="grid gap-12 lg:grid-cols-[300px_1fr] lg:gap-20">
      <div className="hidden lg:block">
        <div className="sticky top-[calc(var(--fsl-header-h)+3rem)]">
          <p className="fsl-eyebrow mb-6">In practice</p>
          <ol className="space-y-4">
            {CAPABILITIES.map((capability, index) => (
              <li key={capability.id}>
                <a
                  href={`#${capability.id}`}
                  className={`flex gap-4 text-[15px] leading-snug transition-colors ${
                    index === activeIndex
                      ? "text-[var(--fsl-ink)]"
                      : "text-[var(--fsl-ink-muted)] hover:text-[var(--fsl-ink)]"
                  }`}
                >
                  <span
                    aria-hidden="true"
                    className={`fsl-mono mt-0.5 text-[11px] ${
                      index === activeIndex ? "text-[var(--fsl-brand-ink)]" : "opacity-40"
                    }`}
                  >
                    {String(index + 1).padStart(2, "0")}
                  </span>
                  {capability.eyebrow}
                </a>
              </li>
            ))}
          </ol>
        </div>
      </div>

      <div>
        {CAPABILITIES.map((capability, index) => (
          <article
            key={capability.id}
            id={capability.id}
            ref={(node) => {
              entryRefs.current[index] = node;
            }}
            className="scroll-mt-32 border-b border-[var(--fsl-line)] py-12 first:pt-0 last:border-b-0 last:pb-0"
          >
              <p className="fsl-eyebrow mb-5 lg:hidden">{capability.eyebrow}</p>
              <h3 className="fsl-display mb-6 max-w-2xl text-[30px] sm:text-[38px] lg:text-[44px]">
                {capability.title}
              </h3>
              <p className="mb-5 max-w-2xl text-[17px] leading-relaxed">{capability.body}</p>
              <p className="mb-6 max-w-2xl text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]">
                {capability.detail}
              </p>
              <a
                href={capability.link.href}
                className="fsl-mono inline-flex items-center gap-2 text-[11px] uppercase tracking-[0.18em] text-[var(--fsl-brand-ink)] transition-opacity hover:opacity-70"
              >
                {capability.link.label}
                <span aria-hidden="true">→</span>
              </a>
          </article>
        ))}
      </div>
    </div>
  );
}
