import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SidRnavDialog } from "./SidRnavDialog";

describe("REMOVE NON RNAV", () => {
  it("does not update RNAV when the menu is only opened and dismissed", () => {
    const onSelectRnav = vi.fn();
    render(<SidRnavDialog open onOpenChange={vi.fn()} sid="" sids={[]} capabilities="NIL" rnavFault={false} onSelectSid={vi.fn()} onSelectRnav={onSelectRnav} onErase={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "ESC" }));
    expect(onSelectRnav).not.toHaveBeenCalled();
  });

  it.each(["2", "5", "10", "NIL"])("updates RNAV 1 immediately for %s without requiring ESC", (capabilities) => {
    const onSelectRnav = vi.fn();
    render(<SidRnavDialog open onOpenChange={vi.fn()} sid="" sids={[]} capabilities={capabilities} rnavFault={false} onSelectSid={vi.fn()} onSelectRnav={onSelectRnav} onErase={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "REMOVE NON RNAV" }));
    expect(onSelectRnav).toHaveBeenCalledExactlyOnceWith("1");
    fireEvent.click(screen.getByRole("button", { name: "ESC" }));
    expect(onSelectRnav).toHaveBeenCalledTimes(1);
  });

  it("disables removal when RNAV 1 is already available", () => {
    render(<SidRnavDialog open onOpenChange={vi.fn()} sid="" sids={[]} capabilities="1" rnavFault={false} onSelectSid={vi.fn()} onSelectRnav={vi.fn()} onErase={vi.fn()} />);
    expect(screen.getByRole("button", { name: "REMOVE NON RNAV" })).toBeDisabled();
  });
});
