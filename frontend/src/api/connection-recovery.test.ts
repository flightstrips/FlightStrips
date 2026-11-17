import {afterEach, describe, expect, it, vi} from "vitest";
import {ConnectionRecovery} from "./connection-recovery";

function fixture() {
  vi.useFakeTimers();
  const callbacks = {send: vi.fn(), fail: vi.fn(), phase: vi.fn(), previousFailure: "socket_closed", attempt: 2};
  const recovery = new ConnectionRecovery(callbacks);
  const welcome = {protocol: 1, connection_id: "physical-1", cf_ray: "ray-1", waiting_for_plugin: false};
  return {callbacks, recovery, welcome};
}
afterEach(() => vi.useRealTimers());

describe("physical connection recovery", () => {
  it("bounds both a stalled upgrade and an open socket without a snapshot", () => {
    const {callbacks, recovery} = fixture();
    vi.advanceTimersByTime(15_000);
    expect(callbacks.fail).toHaveBeenCalledWith("connect_timeout");
    recovery.stop();
    const next = fixture();
    next.recovery.opened();
    vi.advanceTimersByTime(30_000);
    expect(next.callbacks.fail).toHaveBeenCalledWith("snapshot_timeout");
  });

  it("detects lost application acknowledgements even on a ready board", () => {
    const {callbacks, recovery, welcome} = fixture();
    recovery.opened(); recovery.ready(); recovery.receiveHealth(welcome);
    vi.advanceTimersByTime(10_000);
    const probe = callbacks.send.mock.calls[0][0];
    expect(probe).toMatchObject({type: "connection_health", stage: "ready", previous_failure: "socket_closed"});
    recovery.receiveHealth({...welcome, sequence: "obsolete"});
    vi.advanceTimersByTime(30_000);
    expect(callbacks.fail).toHaveBeenCalledOnce();
    expect(callbacks.fail).toHaveBeenCalledWith("heartbeat_timeout");
    expect(vi.getTimerCount()).toBe(0);
  });

  it("keeps an acknowledged waiting connection without reconnecting and bounds later sync", () => {
    const {callbacks, recovery, welcome} = fixture();
    recovery.opened(); recovery.receiveHealth({...welcome, waiting_for_plugin: true});
    for (let i = 0; i < 8; i++) {
      vi.advanceTimersByTime(10_000);
      recovery.receiveHealth({...welcome, waiting_for_plugin: true, sequence: callbacks.send.mock.calls.at(-1)![0].sequence});
    }
    expect(callbacks.fail).not.toHaveBeenCalled();
    vi.advanceTimersByTime(10_000);
    recovery.receiveHealth({...welcome, sequence: callbacks.send.mock.calls.at(-1)![0].sequence});
    expect(callbacks.phase).toHaveBeenLastCalledWith("synchronizing");
    vi.advanceTimersByTime(30_000);
    expect(callbacks.fail).toHaveBeenCalledWith("snapshot_timeout");
  });

  it("requests a snapshot without replaying commands and cancels all old timers", () => {
    const {callbacks, recovery, welcome} = fixture();
    recovery.opened(); recovery.ready();
    expect(recovery.requestSnapshot()).toBe(false); // Compatible with older backends.
    recovery.receiveHealth(welcome);
    expect(recovery.requestSnapshot()).toBe(true);
    expect(callbacks.send).toHaveBeenCalledWith(expect.objectContaining({request_snapshot: true, type: "connection_health"}));
    recovery.stop();
    vi.advanceTimersByTime(120_000);
    recovery.receiveHealth(welcome);
    expect(callbacks.fail).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("does not expire a heartbeat before transmitting it after a suspended tab resumes", () => {
    const {callbacks, recovery, welcome} = fixture();
    recovery.opened(); recovery.ready(); recovery.receiveHealth(welcome);
    vi.setSystemTime(Date.now() + 300_000);
    vi.advanceTimersByTime(10_000);
    expect(callbacks.send).toHaveBeenCalledOnce();
    expect(callbacks.fail).not.toHaveBeenCalled();
    recovery.stop();
  });
});
