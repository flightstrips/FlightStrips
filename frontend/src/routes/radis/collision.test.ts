import {describe, expect, it} from "vitest";
import type {CollisionDetection} from "@dnd-kit/core";
import {radisCollisionDetection} from "./collision";

type Args = Parameters<CollisionDetection>[0];
function fixture(pointerCoordinates: Args["pointerCoordinates"] = {x: 100, y: 350}): Args {
  const rect = (top: number, height: number) => ({top, bottom: top + height, left: 0, right: 400, width: 400, height});
  const panels = [["ACTIVE", rect(0, 300)], ["RUNWAY", rect(300, 200)], ["RADIS_REMOVE", rect(500, 50)]] as const;
  const strips = [["HIDDEN_ACTIVE", rect(300, 100), "ACTIVE"], ["VISIBLE_RUNWAY", rect(300, 100), "RUNWAY"], ["SCROLLED_RUNWAY", rect(200, 100), "RUNWAY"]] as const;
  return {
    active: {id: "DRAGGED", data: {current: {}}, rect: {current: {initial: null, translated: null}}},
    collisionRect: rect(320, 80), pointerCoordinates,
    droppableRects: new Map([...panels.map(([id, r]) => [id, r] as const), ...strips.map(([id, r]) => [id, r] as const)]),
    droppableContainers: [...panels.map(([id, r]) => ({id, key: id, disabled: false, node: {current: null}, rect: {current: r}, data: {current: {}}})), ...strips.map(([id, r, bay]) => ({id, key: id, disabled: false, node: {current: null}, rect: {current: r}, data: {current: {radisBay: bay}}}))],
  };
}

describe("RADIS drops across independently scrolling bays", () => {
  it("ignores an ACTIVE strip clipped behind RUNWAY", () => {
    expect(radisCollisionDetection(fixture())[0]?.id).toBe("VISIBLE_RUNWAY");
  });
  it("targets the empty runway area without snapping into the active bay", () => {
    expect(radisCollisionDetection(fixture({x: 100, y: 450}))[0]?.id).toBe("RUNWAY");
  });
  it("ignores clipped strips when resolving a keyboard drop", () => {
    expect(radisCollisionDetection(fixture(null))[0]?.id).toBe("VISIBLE_RUNWAY");
  });
  it("keeps the delete target reachable", () => {
    expect(radisCollisionDetection(fixture({x: 100, y: 525}))[0]?.id).toBe("RADIS_REMOVE");
  });
});
