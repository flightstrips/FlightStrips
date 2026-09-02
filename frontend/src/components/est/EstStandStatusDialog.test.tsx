import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Bay, type FrontendStrip } from "@/api/models";
import EstStandStatusDialog from "@/components/est/EstStandStatusDialog";

function renderDialog(overrides: Partial<React.ComponentProps<typeof EstStandStatusDialog>> = {}) {
  const props = renderDialogProps(overrides);

  render(<EstStandStatusDialog {...props} />);
  return props;
}

describe("EstStandStatusDialog", () => {
  it("moves the current strip to planned departures directly", () => {
    const props = renderDialog();

    fireEvent.click(screen.getByRole("button", { name: "PLANNED DEP" }));

    expect(props.onPlannedDeparture).toHaveBeenCalledOnce();
    expect(screen.queryByText("No planned departures.")).not.toBeInTheDocument();
  });

  it("offers clearance as a direct command for a planned departure", () => {
    const props = renderDialog({ strip: { callsign: "SAS123", bay: Bay.NotCleared } as FrontendStrip });

    fireEvent.click(screen.getByRole("button", { name: "CLEARED" }));

    expect(props.onCleared).toHaveBeenCalledOnce();
    expect(screen.getByTestId("est-operational-status")).toHaveTextContent("PLANNED DEP");
  });

  it("derives occupied and vacant operational statuses", () => {
    const { rerender } = render(
      <EstStandStatusDialog
        {...renderDialogProps({ strip: undefined, blocked: false })}
      />,
    );
    expect(screen.getByTestId("est-operational-status")).toHaveTextContent("VACANT");

    rerender(
      <EstStandStatusDialog
        {...renderDialogProps({ strip: undefined, blocked: true })}
      />,
    );
    expect(screen.getByTestId("est-operational-status")).toHaveTextContent("OCCUPIED");

    rerender(
      <EstStandStatusDialog
        {...renderDialogProps({ strip: { callsign: "SAS456", bay: Bay.Stand } as FrontendStrip, blocked: false })}
      />,
    );
    expect(screen.getByTestId("est-operational-status")).toHaveTextContent("OCCUPIED");
  });

  it("shows current VGDS and bridge restrictions", () => {
    renderDialog({ stand: "C32" });

    expect(screen.getByText("NO 77W/747/A346/A359")).toBeInTheDocument();
    expect(screen.getByText("NON-SCHENGEN JETWAY")).toBeInTheDocument();
  });

  it("requires confirmation before making an occupied stand vacant", () => {
    const props = renderDialog();

    fireEvent.click(screen.getByRole("button", { name: "VACANT" }));

    expect(props.onVacant).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Yes" }));
    expect(props.onVacant).toHaveBeenCalledOnce();
  });

  it("marks an empty or blocked stand vacant without a deletion warning", () => {
    const props = renderDialog({ strip: undefined });

    fireEvent.click(screen.getByRole("button", { name: "VACANT" }));

    expect(props.onVacant).toHaveBeenCalledOnce();
    expect(screen.queryByText("You want to delete this strip?")).not.toBeInTheDocument();
  });

  it("requires delete confirmation before clearing the flight plan", () => {
    const props = renderDialog();

    fireEvent.click(screen.getByRole("button", { name: "CLEAR FPL" }));

    expect(props.onClearFpl).not.toHaveBeenCalled();
    expect(screen.getByText("You want to delete this strip?")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Yes" }));
    expect(props.onClearFpl).toHaveBeenCalledOnce();
  });
});

function renderDialogProps(overrides: Partial<React.ComponentProps<typeof EstStandStatusDialog>> = {}) {
  return {
    open: true,
    stand: "A12",
    anchor: null,
    strip: { callsign: "SAS123", bay: Bay.Cleared } as FrontendStrip,
    blocked: false,
    onClose: vi.fn(),
    onOccupied: vi.fn(),
    onVacant: vi.fn(),
    onCleared: vi.fn(),
    onClearFpl: vi.fn(),
    onPlannedDeparture: vi.fn(),
    ...overrides,
  } satisfies React.ComponentProps<typeof EstStandStatusDialog>;
}
