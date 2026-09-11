import type {ReactNode, Ref} from "react";

interface AMANWorkspaceShellProps {
  maestro: ReactNode;
  tmt: ReactNode;
  tmtRef?: Ref<HTMLElement>;
}

/**
 * Structural desktop frame for the paired MAESTRO and TMT workspaces.
 *
 * The middle track is intentionally empty: at the reference 3:2 composition
 * it preserves the separation in the design, and on wider screens it absorbs
 * most of the additional width without distorting either operational surface.
 */
export function AMANWorkspaceShell({maestro, tmt, tmtRef}: AMANWorkspaceShellProps) {
  return (
    <main aria-label="Arrival management workspace" className="aman-workspace-page">
      <div className="aman-workspace-shell">
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
