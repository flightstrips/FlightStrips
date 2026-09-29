import {render, screen} from "@testing-library/react";
import {expect, it, vi} from "vitest";
import {CommandOutcome_Status} from "@/api/generated/cluster/v1/storage_pb";
import type {WebSocketState} from "@/store/store";
import {ActionResults} from "./ActionResults";

const state = vi.hoisted(() => ({actionStatuses: {} as Record<string, unknown>, dismissActionStatus: vi.fn()}));
vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (value: WebSocketState) => unknown) => selector(state as unknown as WebSocketState),
}));

it("shows private-message success as local send acceptance", () => {
  state.actionStatuses = {one: {requestId: "one", label: "send_private_message", status: CommandOutcome_Status.SUCCEEDED}};
  render(<ActionResults />);
  expect(screen.getByText("Accepted by local EuroScope send; pilot receipt is not confirmed")).toBeTruthy();
});
