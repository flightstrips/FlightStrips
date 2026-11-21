import {useState} from "react";
import {fireEvent, render, screen} from "@testing-library/react";
import {expect, it, vi} from "vitest";
import {RADISEditDialog} from "./RADISEditDialog";
import type {RADISEdit} from "./RADISStrip";

const state = vi.hoisted(() => ({readOnly: false, boardDefinition: null, availableSids: [], transitionAltitude: 3000}));
vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

function Editor({value, submit}: {value: string; submit: (value?: string) => void}) {
  const [menu, setMenu] = useState<RADISEdit | null>({callsign: "TEST1", field: "route_heading", title: "Route / heading", value});
  return <RADISEditDialog menu={menu} setMenu={setMenu} submit={submit}/>;
}

it.each([
  ["90", "ERASE", "0"],
  ["90", "OK", "0"],
  ["KOK", "ERASE", ""],
  ["KOK", "OK", ""],
])("clears opening value %s with %s after deleting the draft", (value, action, expected) => {
  const submit = vi.fn();
  render(<Editor value={value} submit={submit}/>);
  const input = screen.getByRole("textbox", {name: "Route / heading"});
  fireEvent.change(input, {target: {value: "1"}});
  fireEvent.change(input, {target: {value: ""}});
  fireEvent.click(screen.getByRole("button", {name: action}));
  expect(submit).toHaveBeenCalledWith(expected);
});

it("keeps ERASE tied to an opening route after a numeric draft", () => {
  const submit = vi.fn();
  render(<Editor value="KOK" submit={submit}/>);
  fireEvent.change(screen.getByRole("textbox", {name: "Route / heading"}), {target: {value: "90"}});
  fireEvent.click(screen.getByRole("button", {name: "ERASE"}));
  expect(submit).toHaveBeenCalledWith("");
});
