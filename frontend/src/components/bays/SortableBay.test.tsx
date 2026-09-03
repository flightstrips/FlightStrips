import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SortableStrip } from "./SortableBay";

vi.mock("@dnd-kit/sortable", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@dnd-kit/sortable")>();
  return {
    ...actual,
    useSortable: () => ({
      attributes: {},
      listeners: {},
      setNodeRef: vi.fn(),
      transform: null,
      transition: undefined,
      isDragging: false,
    }),
  };
});

describe("SortableStrip", () => {
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
  });
});
