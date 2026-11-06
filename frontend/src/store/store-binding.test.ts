import {afterEach, describe, expect, it, vi} from "vitest";
import {createWebSocketStore, type WebSocketState} from "./store";
import {Bay, CommunicationType, EventType, type FrontendStrip} from "@/api/models";
import type {WebSocketClient} from "@/api/websocket";

function fixture() {
  const handlers = new Map<EventType, (data: unknown) => void>();
  const client = {on: vi.fn((type, handler) => {handlers.set(type, handler); return () => handlers.delete(type);}), send: vi.fn(() => true), isConnected: () => true, setReadOnly: vi.fn()};
  const store = createWebSocketStore(client as unknown as WebSocketClient);
  store.setState({isInitialized:true,readOnly:false,ownPluginReady:true,airport:"EKCH",amanSessionID:1,position:"118.000",displayedLayout:"CLX",masterCallsign:"OTHER_MASTER",strips:[{callsign:"SAS123",owner:"118.000",bay:Bay.Taxi,next_controllers:["119.000"],communication_type:CommunicationType.Voice}] as FrontendStrip[]});
  return {store,client,handlers};
}
afterEach(() => vi.useRealTimers());

describe("selected-board read-only lifecycle", () => {
  it("retains board data through own-plugin loss and blocks every mutation family", async () => {
    const {store,client,handlers} = fixture();
    const before = store.getState().strips;
    handlers.get(EventType.FrontendDisconnect)!({board:{session_id:1},read_only:true});
    const s = store.getState();
    expect(s).toMatchObject({isInitialized:true,readOnly:true,ownPluginReady:false,airport:"EKCH",masterCallsign:"OTHER_MASTER"});
    expect(s.strips).toBe(before);
    const actions: Array<(s: WebSocketState) => unknown> = [
      s=>s.move("SAS123",Bay.Depart),s=>s.updateOrder("SAS123",null),s=>s.updateStrip("SAS123",{heading:90}),
      s=>s.toggleMarked("SAS123",true),s=>s.setStartReq("SAS123",true),s=>s.setReleasePoint("SAS123","A"),
      s=>s.cdmReady("SAS123"),s=>s.cdmDeicePlatformUpdate("SAS123","P"),s=>s.clxUpdateTobt("SAS123","1200"),
      s=>s.clxOverrideValidation("SAS123","v"),s=>s.issuePdcClearance("SAS123",null),s=>s.revertToVoice("SAS123"),
      s=>s.sendMessage("message",[]),s=>s.sendPrivateMessage("SAS123","message"),s=>s.sendFrequencyMessage("SAS123","message"),
      s=>s.generateSquawk("SAS123"),s=>s.runwayClearance("SAS123"),s=>s.runwayConfirmation("SAS123"),s=>s.resetRunwayClearance("SAS123"),
      s=>s.updateRunwayStatus("04L-22R","CLOSED"),s=>s.missedApproach("SAS123"),s=>s.transferStrip("SAS123"),
      s=>s.assumeStrip("SAS123"),s=>s.forceAssumeStrip("SAS123"),s=>s.freeStrip("SAS123"),s=>s.cancelTransfer("SAS123"),
      s=>s.requestTag("SAS123"),s=>s.acceptTagRequest("SAS123"),s=>s.selectNextController("SAS123","TW"),s=>s.startRequestAndTransfer("SAS123"),
      s=>s.occupyStand("A1"),s=>s.vacateStand("A1",1,1),s=>s.requestManualStand("SAS123","A1",1),s=>s.requestAutomaticStand("SAS123",1),
      s=>s.confirmStandOverride("SAS123","A1",1,"reason"),s=>s.acknowledgeStandAssignment("SAS123",1),
      s=>s.createManualFPL("NEW123","ESSA","SID","2000","1200","A320","250","DCT","A1","22R"),
      s=>s.createVFRFPL("NEW123","C172",1,"7000","VFR","EN",""),s=>s.deleteTacticalStrip(1),s=>s.confirmTacticalStrip(1),
      s=>s.forceAssumeTacticalStrip(1),s=>s.markTacticalStrip(1,true),s=>s.startTacticalTimer(1),s=>s.moveTacticalStrip(1,null,Bay.Taxi),
      s=>s.acknowledgeValidationStatus("SAS123","v"),s=>s.acknowledgeUnexpectedChange("SAS123","heading"),
    ];
    for (const action of actions) await action(s);
    expect(client.send).not.toHaveBeenCalled();
    expect(store.getState().strips).toBe(before);
    store.dispose();
  });
  it("cancels deferred frequency work and force-assume continuations when the binding changes", async () => {
    vi.useFakeTimers();
    const {store,client,handlers} = fixture();
    store.setState({displayedLayout:"EST",strips:[{...store.getState().strips[0],owner:"OTHER"}]});
    const message = store.getState().sendFrequencyMessage("SAS123","Hold position");
    const transfer = store.getState().startRequestAndTransfer("SAS123");
    expect(client.send).toHaveBeenCalledTimes(2);
    const requests = client.send.mock.calls.map(call => (call as unknown as [Record<string,unknown>])[0]);
    store.suspendBoard();
    expect(await message).toMatchObject({submitted:false,cancelled:true});
    expect(await transfer).toBe(false);
    handlers.get(EventType.FrontendFrequencyMessageResult)!({request_id:requests[0].request_id,callsign:"SAS123",submitted:true,applied_actions:1});
    await vi.advanceTimersByTimeAsync(15000);
    expect(client.send).toHaveBeenCalledTimes(2);
    store.dispose();
    expect(handlers.size).toBe(0);
  });
  it("a disposed store cannot affect a newly created board or show delayed socket side effects", () => {
    const old = fixture();
    const staleHandler = old.handlers.get(EventType.FrontendStripUpdate)!;
    old.store.dispose();
    const next = fixture();
    staleHandler({callsign:"OLD123"});
    old.store.getState().toggleMarked("SAS123",true);
    expect(old.client.send).not.toHaveBeenCalled();
    expect(next.store.getState().strips.map(s=>s.callsign)).toEqual(["SAS123"]);
    next.store.dispose();
  });
});
