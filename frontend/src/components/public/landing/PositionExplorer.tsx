import { useRef, useState, type KeyboardEvent } from "react";
import { POSITIONS, type PositionKey, type PositionPanel } from "./content";
import { ArrivalSpecimen, ClearedSpecimen } from "./StripSpecimen";

function PanelBody({ panel }: { panel: PositionPanel }) {
  return (
    <div className="fsl-position-body">
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

      <div className="fsl-position-examples">
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
    if (delta === 0 && event.key !== "Home" && event.key !== "End") return;

    event.preventDefault();
    const next = POSITIONS[event.key === "Home" ? 0 : event.key === "End" ? POSITIONS.length - 1 : (index + delta + POSITIONS.length) % POSITIONS.length];
    setActive(next.key);
    setOpenMobile(next.key);
    tabRefs.current[next.key]?.focus();
  };

  return (
    <>
      {/* Desktop: tab list beside the active panel. */}
      <div className="fsl-position-explorer hidden gap-8 lg:grid lg:grid-cols-[200px_1fr] xl:gap-12">
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
                onClick={() => { setActive(panel.key); setOpenMobile(panel.key); }}
                onKeyDown={(event) => onTabKeyDown(event, index)}
                className={`border-b border-[var(--fsl-line)] py-4 text-left text-[15px] transition-colors first:border-t ${
                  selected
                    ? "text-[var(--fsl-ink)]"
                    : "text-[var(--fsl-ink-muted)] hover:text-[var(--fsl-ink)]"
                }`}
              >
                <span
                  aria-hidden="true"
                  className={`fsl-mono mr-3 text-xs ${selected ? "text-[var(--fsl-brand-ink)]" : "text-[var(--fsl-ink-muted)]"}`}
                >
                  {String(index + 1).padStart(2, "0")}
                </span>
                {panel.tab}
              </button>
            );
          })}
        </div>

        <div className="fsl-position-panels">
          {POSITIONS.map((panel) => (
            <div
              key={panel.key}
              role="tabpanel"
              id={`fsl-panel-${panel.key}`}
              aria-labelledby={`fsl-tab-${panel.key}`}
              aria-hidden={panel.key !== activePanel.key}
              inert={panel.key !== activePanel.key}
              tabIndex={panel.key === activePanel.key ? 0 : -1}
              className="fsl-position-panel"
            >
              <PanelBody panel={panel} />
            </div>
          ))}
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
                  onClick={() => { setOpenMobile(open ? null : panel.key); setActive(panel.key); }}
                  className="flex w-full items-center justify-between gap-4 py-5 text-left text-[17px]"
                >
                  <span>
                    <span aria-hidden="true" className="fsl-mono mr-3 text-xs text-[var(--fsl-ink-muted)]">
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
