import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Bay, type FrontendStrip } from "@/api/models";
import { useSortable } from "@dnd-kit/sortable";
import { SortableBay, SortableStrip } from "./SortableBay";

const stripTransfers = vi.hoisted(() => ({} as Record<string, { from: string; to: string; isTagRequest: boolean }>));

vi.mock("@/store/store-hooks", () => ({
  useMyPosition: () => "118.105",
  useOpenValidationDialog: () => vi.fn(),
  useStripTransfers: () => stripTransfers,
}));

vi.mock("@dnd-kit/sortable", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@dnd-kit/sortable")>();
  return {
    ...actual,
    useSortable: vi.fn(() => ({
      attributes: {},
      listeners: {},
      setNodeRef: vi.fn(),
      transform: null,
      transition: undefined,
      isDragging: false,
    })),
  };
});

describe("SortableStrip", () => {
  it("locks a strip during an active non-tag handover", () => {
    const strip = { callsign: "SAS123", bay: Bay.Cleared, owner: "118.105" } as FrontendStrip;
    stripTransfers.SAS123 = { from: "118.105", to: "121.905", isTagRequest: false };
    vi.mocked(useSortable).mockClear();

    render(
      <SortableBay strips={[strip]}>
        {() => <span>{strip.callsign}</span>}
      </SortableBay>,
    );

    expect(useSortable).toHaveBeenLastCalledWith(expect.objectContaining({
      id: "SAS123",
      disabled: true,
    }));
    delete stripTransfers.SAS123;
  });

  it.each([Bay.NotCleared, Bay.Cleared])("locks dragging only for planned departures without a handover (%s)", (bay) => {
    const strip = { callsign: "SAS123", bay } as FrontendStrip;
    vi.mocked(useSortable).mockClear();

    render(
      <SortableBay strips={[strip]}>
        {() => <span>{strip.callsign}</span>}
      </SortableBay>,
    );

    expect(useSortable).toHaveBeenLastCalledWith(expect.objectContaining({
      id: "SAS123",
      disabled: bay === Bay.NotCleared,
    }));
  });

  it("opens an active validation problem before child strip interactions run", () => {
    const onValidationClick = vi.fn();
    const onChildClick = vi.fn();

    render(
      <SortableStrip
        callsign="SAS123"
        validationBlocked
        onValidationClick={onValidationClick}
      >
        <button onClick={onChildClick}>SAS123</button>
      </SortableStrip>,
    );

    fireEvent.click(screen.getByRole("button", { name: "SAS123" }));

    expect(onValidationClick).toHaveBeenCalledWith("SAS123");
    expect(onChildClick).not.toHaveBeenCalled();
  });

  it("keeps native touch scrolling available on an enabled strip", () => {
    render(
      <SortableStrip callsign="SAS456">
        <span>SAS456</span>
      </SortableStrip>,
    );

    expect(screen.getByText("SAS456").parentElement?.style.touchAction).toBe("auto");
    expect(screen.getByText("SAS456").parentElement?.style.cursor).toBe("pointer");
  });

  it("shows no cursor on a strip that cannot be dragged", () => {
    render(
      <SortableStrip callsign="SAS789" dragDisabled>
        <span>SAS789</span>
      </SortableStrip>,
    );

    expect(screen.getByText("SAS789").parentElement?.style.cursor).toBe("");
  });

  it("keeps the hand cursor when clicking opens a validation problem", () => {
    render(
      <SortableStrip callsign="SAS012" validationBlocked onValidationClick={vi.fn()}>
        <span>SAS012</span>
      </SortableStrip>,
    );

    expect(screen.getByText("SAS012").parentElement?.style.cursor).toBe("pointer");
  });
});
