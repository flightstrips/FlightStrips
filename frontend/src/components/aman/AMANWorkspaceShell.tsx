import type {CSSProperties, ReactNode, Ref} from "react";

interface AMANWorkspaceShellProps {
  accCompactHoldingCount?: number;
  maestro: ReactNode;
  tmt: ReactNode;
  tmtRef?: Ref<HTMLElement>;
}

/**
 * Structural desktop frame for the paired MAESTRO and TMT workspaces.
 *
 * The middle track is intentionally empty: at the reference 3:2 composition
 * it preserves separation, and on wider screens absorbs extra width. Compact
 * ACC layouts instead keep TMT at the right edge while MAESTRO expands.
 */
export function AMANWorkspaceShell({accCompactHoldingCount, maestro, tmt, tmtRef}: AMANWorkspaceShellProps) {
  const compactLayout = accCompactHoldingCount !== undefined;
  return (
    <main aria-label="Arrival management workspace" className="aman-workspace-page">
      <div
        className={`aman-workspace-shell${compactLayout ? " aman-workspace-shell-acc-compact" : ""}`}
        style={compactLayout ? {"--aman-acc-holding-count": accCompactHoldingCount} as CSSProperties : undefined}
      >
        <section aria-label="MAESTRO sequence workspace" className="aman-workspace-maestro">
          {maestro}
        </section>

        <div aria-hidden="true" className="aman-workspace-separator" />

        <aside
          aria-label="TMT analysis area"
          className="aman-workspace-tmt"
          ref={tmtRef}
          tabIndex={-1}
        >
          {tmt}
        </aside>
      </div>
    </main>
  );
}
