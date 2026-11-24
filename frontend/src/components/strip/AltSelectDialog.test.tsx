import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { AltSelectDialog } from "./AltSelectDialog";

vi.mock("@/store/store-hooks", () => ({ useTransitionAltitude: () => 5000 }));

it.each([4950, 5100])("rejects a fractional flight level and accepts %i feet", altitude => {
  const onSelect = vi.fn();
  const onOpenChange = vi.fn();
  render(<AltSelectDialog open value={undefined} onOpenChange={onOpenChange} onSelect={onSelect} />);
  const input = screen.getByRole("textbox");

  fireEvent.change(input, { target: { value: "5050" } });
  fireEvent.keyDown(input, { key: "Enter" });
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(input).toHaveAccessibleDescription(/whole flight level/);
  expect(onSelect).not.toHaveBeenCalled();
  expect(onOpenChange).not.toHaveBeenCalled();

  fireEvent.change(input, { target: { value: String(altitude) } });
  fireEvent.keyDown(input, { key: "Enter" });
  expect(onSelect).toHaveBeenCalledExactlyOnceWith(altitude);
  expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false);
});
