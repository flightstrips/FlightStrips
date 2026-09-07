import { useRef, useState, type KeyboardEvent } from "react";
import { POSITIONS, type PositionKey, type PositionPanel } from "./content";
import { ArrivalSpecimen, ClearedSpecimen } from "./StripSpecimen";

function PanelBody({ panel }: { panel: PositionPanel }) {
  return (
    <div className="grid gap-10 lg:grid-cols-2">
      <div>
        <h3 className="fsl-display mb-4 text-[28px] lg:text-[34px]">{panel.title}</h3>
        <p className="mb-8 max-w-lg text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]">
          {panel.summary}
        </p>

        <h4 className="fsl-eyebrow mb-4">Shipped</h4>
        <ul className="space-y-2.5">
          {panel.capabilities.map((capability) => (
            <li key={capability} className="flex gap-3 text-sm leading-relaxed">
              <span aria-hidden="true" className="mt-[7px] h-1 w-1 shrink-0 bg-[var(--fsl-brand-ink)]" />
              <span>{capability}</span>
            </li>
          ))}
        </ul>

        <div className="mt-8 border-t border-[var(--fsl-line)] pt-5">
          <h4 className="fsl-eyebrow mb-2">{panel.layoutLabel}</h4>
          <p className="text-sm leading-relaxed text-[var(--fsl-ink-muted)]">{panel.layout}</p>
        </div>
      </div>

      <div className="flex flex-col justify-center gap-7 border border-[var(--fsl-line)] bg-[var(--fsl-surface)] p-6 sm:p-8">
        <p className="fsl-eyebrow">On the board</p>
        {panel.strips.map((strip) => (
          <div key={strip.caption}>
            {strip.variant === "cleared" ? (
              <ClearedSpecimen
                si={strip.si}
                nextLabel={strip.nextLabel}
                callsign={strip.callsign}
                height={54}
              />
            ) : (
              <ArrivalSpecimen
                si={strip.si}
                nextLabel={strip.nextLabel}
                callsign={strip.callsign}
                height={54}
              />
            )}
            <p className="mt-2.5 text-xs leading-relaxed text-[var(--fsl-ink-muted)]">{strip.caption}</p>
          </div>
        ))}
      </div>
    </div>
  );
}

export function PositionExplorer() {
  const [active, setActive] = useState<PositionKey>("clr");
  const [openMobile, setOpenMobile] = useState<PositionKey | null>("clr");
  const tabRefs = useRef<Partial<Record<PositionKey, HTMLButtonElement | null>>>({});

  const activePanel = POSITIONS.find((panel) => panel.key === active) ?? POSITIONS[0];

  const onTabKeyDown = (event: KeyboardEvent, index: number) => {
    const delta = event.key === "ArrowDown" ? 1 : event.key === "ArrowUp" ? -1 : 0;
    if (delta === 0) return;

    event.preventDefault();
    const next = POSITIONS[(index + delta + POSITIONS.length) % POSITIONS.length];
    setActive(next.key);
    tabRefs.current[next.key]?.focus();
  };

  return (
    <>
      {/* Desktop: tab list beside the active panel. */}
      <div className="hidden gap-12 lg:grid lg:grid-cols-[240px_1fr]">
        <div role="tablist" aria-label="Controller positions" aria-orientation="vertical" className="flex flex-col">
          {POSITIONS.map((panel, index) => {
            const selected = panel.key === active;
            return (
              <button
                key={panel.key}
                ref={(node) => {
                  tabRefs.current[panel.key] = node;
                }}
                role="tab"
                id={`fsl-tab-${panel.key}`}
                aria-selected={selected}
                aria-controls={`fsl-panel-${panel.key}`}
                tabIndex={selected ? 0 : -1}
                onClick={() => setActive(panel.key)}
                onKeyDown={(event) => onTabKeyDown(event, index)}
                className={`border-b border-[var(--fsl-line)] py-4 text-left text-[15px] transition-colors first:border-t ${
                  selected
                    ? "font-medium text-[var(--fsl-ink)]"
                    : "text-[var(--fsl-ink-muted)] hover:text-[var(--fsl-ink)]"
                }`}
              >
                <span
                  aria-hidden="true"
                  className={`fsl-mono mr-3 text-[11px] ${selected ? "text-[var(--fsl-brand-ink)]" : "opacity-40"}`}
                >
                  {String(index + 1).padStart(2, "0")}
                </span>
                {panel.tab}
              </button>
            );
          })}
        </div>

        <div
          role="tabpanel"
          id={`fsl-panel-${activePanel.key}`}
          aria-labelledby={`fsl-tab-${activePanel.key}`}
          tabIndex={0}
        >
          <PanelBody panel={activePanel} />
        </div>
      </div>

      {/* Mobile: the same content as an accordion. */}
      <div className="lg:hidden">
        {POSITIONS.map((panel, index) => {
          const open = openMobile === panel.key;
          return (
            <div key={panel.key} className="border-b border-[var(--fsl-line)] first:border-t">
              <h3>
                <button
                  type="button"
                  aria-expanded={open}
                  aria-controls={`fsl-acc-${panel.key}`}
                  onClick={() => setOpenMobile(open ? null : panel.key)}
                  className="flex w-full items-center justify-between gap-4 py-5 text-left text-[17px]"
                >
                  <span>
                    <span aria-hidden="true" className="fsl-mono mr-3 text-[11px] opacity-40">
                      {String(index + 1).padStart(2, "0")}
                    </span>
                    {panel.tab}
                  </span>
                  <span aria-hidden="true" className="fsl-mono text-[var(--fsl-brand-ink)]">
                    {open ? "−" : "+"}
                  </span>
                </button>
              </h3>
              <div id={`fsl-acc-${panel.key}`} hidden={!open} className="pb-10">
                <PanelBody panel={panel} />
              </div>
            </div>
          );
        })}
      </div>
    </>
  );
}
