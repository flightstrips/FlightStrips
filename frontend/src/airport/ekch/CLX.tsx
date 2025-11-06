
import { FlightStrip } from "@/components/strip/FlightStrip.tsx";
import { Message } from "@/components/Message.tsx";
import {useClearedStrips, useNorwegianBayStrips, useOtherBayStrips, useSasBayStrips} from "@/store/ekch.ts";
import { useWebSocketStore } from "@/store/store-provider.tsx";
import type {FrontendStrip} from "@/api/models.ts";
import React, { useMemo, useState } from "react";

export default function DEL() {
  const sasStrips = useSasBayStrips().sort((a, b) => a.sequence - b.sequence);
  const norgewianStrips = useNorwegianBayStrips().sort((a, b) => a.sequence - b.sequence);
  const otherStrips = useOtherBayStrips().sort((a, b) => a.sequence - b.sequence);
  const cleared = useClearedStrips().sort((a, b) => a.sequence - b.sequence);

  const updateOrder = useWebSocketStore(state => state.updateOrder);

  const [draggingCallsign, setDraggingCallsign] = useState<string | null>(null);
  const [dropAfterCallsign, setDropAfterCallsign] = useState<string | null>(null);
  const [hoveredHeaderKey, setHoveredHeaderKey] = useState<string | null>(null);

  // Map of callsign -> strip to render a full inline preview while dragging
  const stripByCallsign = useMemo(() => {
    const m = new Map<string, FrontendStrip>();
    for (const s of [...sasStrips, ...norgewianStrips, ...otherStrips, ...cleared]) {
      m.set(s.callsign, s);
    }
    return m;
  }, [sasStrips, norgewianStrips, otherStrips, cleared]);

  const onDragStartStrip = (e: React.DragEvent, callsign: string) => {
    e.dataTransfer.setData('text/plain', callsign);
    // give visual feedback
    e.dataTransfer.effectAllowed = 'move';
    setDraggingCallsign(callsign);
    // Use browser default drag image to ensure immediate visual feedback
  };


  const onDragOverStrip = (e: React.DragEvent, afterCallsign: string) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (draggingCallsign && draggingCallsign !== afterCallsign) {
      setDropAfterCallsign(afterCallsign);
    }
  };

  const clearDnDState = () => {
    setDraggingCallsign(null);
    setDropAfterCallsign(null);
    setHoveredHeaderKey(null);
  };

  const onDragOverList = (e: React.DragEvent, list: FrontendStrip[], key?: string) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (!draggingCallsign) return;
    if (list.length > 0) {
      setHoveredHeaderKey(null);
      const last = list[list.length - 1];
      if (last.callsign !== draggingCallsign) {
        setDropAfterCallsign(last.callsign);
      }
    } else if (key) {
      // empty list: show intention to place at top
      setDropAfterCallsign(null);
      setHoveredHeaderKey(key);
    }
  };

  const onDragOverHeader = (e: React.DragEvent, key: string) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    // show intention to drop at top by clearing after-target indicator
    setDropAfterCallsign(null);
    setHoveredHeaderKey(key);
  };

  const dropOnHeaderTop = (e: React.DragEvent) => {
    e.preventDefault();
    const callsign = e.dataTransfer.getData('text/plain');
    if (!callsign) return;
    updateOrder(callsign, null); // move to very top
    clearDnDState();
  };

  const dropOnListEnd = (e: React.DragEvent, list: FrontendStrip[]) => {
    e.preventDefault();
    const callsign = e.dataTransfer.getData('text/plain');
    if (!callsign) return;
    if (list.length === 0) {
      updateOrder(callsign, null);
      clearDnDState();
      return;
    }
    const last = list[list.length - 1];
    if (last.callsign === callsign) return; // already last
    updateOrder(callsign, last.callsign); // place after last
    clearDnDState();
  };

  const dropAfterStrip = (e: React.DragEvent, afterCallsign: string) => {
    e.preventDefault();
    const callsign = e.dataTransfer.getData('text/plain');
    if (!callsign || callsign === afterCallsign) return;
    updateOrder(callsign, afterCallsign); // place after this strip
    clearDnDState();
  };

  const DragPreview = ({ status }: { status: string }) => {
    if (!draggingCallsign) return null;
    const s = stripByCallsign.get(draggingCallsign);
    if (!s) return null;
    return (
      <div className="flex items-center gap-1 opacity-50 pointer-events-none">
        <div className="flex-1">
          <FlightStrip
            callsing={s.callsign}
            destination={s.destination}
            stand={s.stand}
            eobt={s.eobt}
            status={status}
          />
        </div>
      </div>
    );
  };

  const ReorderableStrip = (
    { list, index, status }: { list: FrontendStrip[]; index: number; status: string }
  ) => {
    const strip = list[index];

    const isDragging = draggingCallsign === strip.callsign;
    const showAfter = dropAfterCallsign === strip.callsign && draggingCallsign && draggingCallsign !== strip.callsign;

    return (
      <>
        <div
          key={strip.callsign}
          className={`flex items-center gap-1 select-none ${isDragging ? 'opacity-50' : ''}`}
          draggable
          onDragStart={(e) => onDragStartStrip(e, strip.callsign)}
          onDragOver={(e) => onDragOverStrip(e, strip.callsign)}
          onDrop={(e) => dropAfterStrip(e, strip.callsign)}
          onDragEnd={clearDnDState}
        >
          <div className="flex-1">
            <FlightStrip
              callsing={strip.callsign}
              destination={strip.destination}
              stand={strip.stand}
              eobt={strip.eobt}
              status={status}
            />
          </div>
        </div>
        {showAfter && (
          <DragPreview status={status} />
        )}
      </>
    );
  };

  return (
    <>
      <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2 aspect-video">
        <div className="w-1/4 h-full bg-[#555355]">
          <div
            className="bg-[#393939] h-10 flex items-center px-2 justify-between"
            onDragOver={(e) => onDragOverHeader(e, 'others')}
            onDrop={dropOnHeaderTop}
          >
            <span className="text-white font-bold text-lg">
              OTHERS
            </span>
            <span className="flex gap-2">
              <button className="bg-[#646464] text-white font-bold text-lg px-4 border-2 border-white active:bg-[#424242]">
                NEW
              </button>
              <button className="bg-[#646464] text-white font-bold text-lg px-4 border-2 border-white active:bg-[#424242]">
                PLANNED
              </button>
            </span>
          </div>
          <div
            className="h-[calc(100%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
            onDragOver={(e) => onDragOverList(e, otherStrips, 'others')}
            onDrop={(e) => dropOnListEnd(e, otherStrips)}
          >
            {hoveredHeaderKey === 'others' && draggingCallsign && (
              <DragPreview status="CLR" />
            )}
            {otherStrips.map((_, i) => (
              <ReorderableStrip list={otherStrips} index={i} status="CLR" key={otherStrips[i].callsign} />
            ))}
          </div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div
            className="bg-[#393939] h-10 flex items-center px-2 justify-between"
            onDragOver={(e) => onDragOverHeader(e, 'sas')}
            onDrop={dropOnHeaderTop}
          >
            <span className="text-white font-bold text-lg">
              SAS
            </span>
          </div>
          <div
            className="h-[calc(50%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
            onDragOver={(e) => onDragOverList(e, sasStrips, 'sas')}
            onDrop={(e) => dropOnListEnd(e, sasStrips)}
          >
            {hoveredHeaderKey === 'sas' && draggingCallsign && (
              <DragPreview status="CLR" />
            )}
            {sasStrips.map((_, i) => (
              <ReorderableStrip list={sasStrips} index={i} status="CLR" key={sasStrips[i].callsign} />
            ))}
          </div>
          <div
            className="bg-[#393939] h-10 flex items-center px-2 justify-between"
            onDragOver={(e) => onDragOverHeader(e, 'norwegian')}
            onDrop={dropOnHeaderTop}
          >
            <span className="text-white font-bold text-lg">
              NORWEGIAN
            </span>
          </div>
          <div
            className="h-[calc(50%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
            onDragOver={(e) => onDragOverList(e, norgewianStrips, 'norwegian')}
            onDrop={(e) => dropOnListEnd(e, norgewianStrips)}
          >
            {hoveredHeaderKey === 'norwegian' && draggingCallsign && (
              <DragPreview status="CLR" />
            )}
            {norgewianStrips.map((_, i) => (
              <ReorderableStrip list={norgewianStrips} index={i} status="CLR" key={norgewianStrips[i].callsign} />
            ))}
          </div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div
            className="bg-[#393939] h-10 flex items-center px-2 justify-between"
            onDragOver={(e) => onDragOverHeader(e, 'cleared')}
            onDrop={dropOnHeaderTop}
          >
            <span className="text-gray-100 font-bold text-lg">
              CLEARED
            </span>
          </div>
          <div
            className="h-1/2 w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
            onDragOver={(e) => onDragOverList(e, cleared, 'cleared')}
            onDrop={(e) => dropOnListEnd(e, cleared)}
          >
            {hoveredHeaderKey === 'cleared' && draggingCallsign && (
              <DragPreview status="CLROK" />
            )}
            {cleared.map((_, i) => (
              <ReorderableStrip list={cleared} index={i} status="CLROK" key={cleared[i].callsign} />
            ))}
          </div>
          <div className="bg-primary h-10 flex items-center px-2 justify-between">
            <span className="text-gray-100 font-bold text-lg">
              MESSAGES
            </span>
          </div>
          <div className="h-[calc(50%-6rem)] w-full bg-[#555355]">
            <Message>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Nam tincidunt vitae enim eget porttitor. Suspendisse ultrices ullamcorper tortor, vitae condimentum lacus convallis at. </Message>
            <Message><b>FLIGHTSTRIPS</b> has deteced that EKCH_DEL has logged off. You are in change of Delivery!</Message>
            <Message>VFR Request LOW PASS rwy 12</Message>
          </div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
            <span className="text-[#393939] font-bold text-lg">
              PUSHBACK
            </span>
          </div>
          <div className="h-2/5 w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
          </div>
          <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
            <span className="text-[#393939] font-bold text-lg">
              TWY DEP
            </span>
          </div>
          <div className="h-[calc(60%-5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
          </div>
        </div>
      </div>
      
    </>
  );
}
