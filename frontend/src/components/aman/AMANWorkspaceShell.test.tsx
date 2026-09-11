import {createRef} from "react";
import {render, screen} from "@testing-library/react";
import {describe, expect, it} from "vitest";

import {AMANWorkspaceShell} from "./AMANWorkspaceShell";

describe("AMANWorkspaceShell", () => {
  it("exposes the MAESTRO and TMT workspaces as named landmarks", () => {
    const tmtRef = createRef<HTMLElement>();

    render(
      <AMANWorkspaceShell
        maestro={<div>Sequence board</div>}
        tmt={<div>Analysis tools</div>}
        tmtRef={tmtRef}
      />,
    );

    expect(screen.getByRole("main", {name: "Arrival management workspace"})).toBeInTheDocument();
    expect(screen.getByRole("region", {name: "MAESTRO sequence workspace"})).toHaveTextContent("Sequence board");
    expect(screen.getByRole("complementary", {name: "TMT analysis area"})).toHaveTextContent("Analysis tools");
    expect(tmtRef.current).toBe(screen.getByRole("complementary", {name: "TMT analysis area"}));
  });
});
