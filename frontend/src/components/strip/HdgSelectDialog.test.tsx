import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { HdgSelectDialog } from "./HdgSelectDialog";

describe("HdgSelectDialog", () => {
  it.each([
    ["22R", "217", 217],
    ["04L", "027", 27],
    ["30", "299", 299],
    ["12", "119", 119],
  ])("fills the runway-specific SA heading for runway %s", (runway, displayValue, heading) => {
    const onSelect = vi.fn();
    render(
      <HdgSelectDialog
        open
        onOpenChange={vi.fn()}
        value={undefined}
        runway={runway}
        onSelect={onSelect}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "SA" }));
    expect(screen.getByRole("textbox", { name: "Heading" })).toHaveValue(displayValue);
    fireEvent.click(screen.getByRole("button", { name: "OK" }));
    expect(onSelect).toHaveBeenCalledWith(heading);
  });

  it("keeps LT/RT as a UI-only direction while selecting a numeric heading", () => {
    const onSelect = vi.fn();
    const onTurnDirectionChange = vi.fn();
    render(
      <HdgSelectDialog
        open
        onOpenChange={vi.fn()}
        value={undefined}
        onSelect={onSelect}
        onTurnDirectionChange={onTurnDirectionChange}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "LT" }));
    expect(onSelect).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "2" }));
    fireEvent.click(screen.getByRole("button", { name: "7" }));
    fireEvent.click(screen.getByRole("button", { name: "0" }));
    fireEvent.click(screen.getByRole("button", { name: "OK" }));

    expect(onTurnDirectionChange).toHaveBeenCalledWith("left");
    expect(onSelect).toHaveBeenCalledWith(270);
  });

  it("allows a zero heading", () => {
    const onSelect = vi.fn();
    render(
      <HdgSelectDialog
        open
        onOpenChange={vi.fn()}
        value={undefined}
        onSelect={onSelect}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "0" }));
    fireEvent.click(screen.getByRole("button", { name: "00" }));
    fireEvent.click(screen.getByRole("button", { name: "OK" }));

    expect(onSelect).toHaveBeenCalledWith(0);
  });

  it("removes the last digit with the backspace key", () => {
    render(
      <HdgSelectDialog
        open
        onOpenChange={vi.fn()}
        value={undefined}
        onSelect={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "2" }));
    fireEvent.click(screen.getByRole("button", { name: "7" }));
    fireEvent.click(screen.getByRole("button", { name: "Backspace" }));

    expect(screen.getByRole("textbox", { name: "Heading" })).toHaveValue("2");
  });

  it.each([
    ["LT350", "350", "left"],
    ["LT360", "360", "left"],
    ["RT090", "090", "right"],
  ] as const)("selects the %s quick preset", (preset, heading, direction) => {
    const onSelect = vi.fn();
    const onTurnDirectionChange = vi.fn();
    render(
      <HdgSelectDialog
        open
        onOpenChange={vi.fn()}
        value={undefined}
        onSelect={onSelect}
        onTurnDirectionChange={onTurnDirectionChange}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: preset }));
    expect(screen.getByRole("textbox", { name: "Heading" })).toHaveValue(heading);
    fireEvent.click(screen.getByRole("button", { name: "OK" }));

    expect(onSelect).toHaveBeenCalledWith(Number(heading));
    expect(onTurnDirectionChange).toHaveBeenCalledWith(direction);
  });

  it("requires OK to commit erase and leaves the heading unchanged on ESC", () => {
    const onSelect = vi.fn();
    const onTurnDirectionChange = vi.fn();
    const onOpenChange = vi.fn();
    render(
      <HdgSelectDialog
        open
        onOpenChange={onOpenChange}
        value={270}
        turnDirection="left"
        onSelect={onSelect}
        onTurnDirectionChange={onTurnDirectionChange}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "ERASE" }));
    expect(screen.getByRole("textbox", { name: "Heading" })).toHaveValue("");
    expect(onSelect).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "OK" }));
    expect(onSelect).toHaveBeenCalledWith(undefined);
    expect(onTurnDirectionChange).toHaveBeenCalledWith(undefined);

    onSelect.mockClear();
    fireEvent.click(screen.getByRole("button", { name: "ESC" }));
    expect(onSelect).not.toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenLastCalledWith(false);
  });
});
