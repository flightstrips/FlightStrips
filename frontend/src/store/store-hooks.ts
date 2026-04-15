import { useContext } from 'react';
import { useStore } from 'zustand';
import type { FrontendController } from '@/api/models.ts';
import { type WebSocketState } from './store.ts';
import { WebSocketStoreContext } from './store-context.ts';
export { useUserRating } from './user-rating-context.ts';

const DELIVERY_SECTORS = ['DEL'];
const APRON_SECTORS = ['AA', 'AD'];
const TOWER_SECTORS = ['TE', 'TW', 'GW'];

const normalizeOwnedSector = (sector: string) => {
  if (sector === 'GWA' || sector === 'GWD') {
    return 'GW';
  }

  return sector;
};

const ownsAnySector = (controller: FrontendController | undefined, sectors: string[]) => {
  if (!controller) {
    return false;
  }

  const ownedSectors = new Set((controller.owned_sectors ?? []).map(normalizeOwnedSector));
  return sectors.some((sector) => ownedSectors.has(sector));
};

const controllerOwnsDeliverySector = (controller: FrontendController | undefined) =>
  ownsAnySector(controller, DELIVERY_SECTORS) || controller?.section === 'DEL';

const controllerOwnsApronSector = (controller: FrontendController | undefined) =>
  ownsAnySector(controller, APRON_SECTORS) || controller?.section === 'GND';

export const controllerOwnsTowerSectors = (controller: FrontendController | undefined) =>
  ownsAnySector(controller, TOWER_SECTORS) || controller?.section === 'TWR';

export const useWebSocketStore = <T,>(selector: (state: WebSocketState) => T): T => {
  const store = useContext(WebSocketStoreContext);

  if (!store) {
    throw new Error('useWebSocketStore must be used within a WebSocketStoreProvider');
  }

  return useStore(store, selector);
};

export const useControllers = () => useWebSocketStore((state) => state.controllers);
export const useStrips = () => useWebSocketStore((state) => state.strips);
export const useTacticalStrips = () => useWebSocketStore((state) => state.tacticalStrips);
export const usePosition = () => useWebSocketStore((state) => state.position);
export const useAirport = () => useWebSocketStore((state) => state.airport);
export const useCallsign = () => useWebSocketStore((state) => state.callsign);
export const useRunwaySetup = () => useWebSocketStore((state) => state.runwaySetup);
export const useStrip = (callsign: string) => useWebSocketStore((state) => state.strips.find(strip => strip.callsign === callsign));
export const useSelectedCallsign = () => useWebSocketStore((state) => state.selectedCallsign);
export const useSelectStrip = () => useWebSocketStore((state) => state.selectStrip);
export const useTagRequestArmed = () => useWebSocketStore((state) => state.tagRequestArmed);
export const useSetTagRequestArmed = () => useWebSocketStore((state) => state.setTagRequestArmed);
export const useMarkArmed = () => useWebSocketStore((state) => state.markArmed);
export const useSetMarkArmed = () => useWebSocketStore((state) => state.setMarkArmed);
export const useMessages = () => useWebSocketStore((state) => state.messages);
/** @deprecated use useMessages */
export const useActiveMessages = () => useWebSocketStore((state) => state.messages);
export const useMyPosition = () => useWebSocketStore((state) => state.position);
export const useStripTransfers = () => useWebSocketStore((state) => state.stripTransfers);
export const useMetar = () => useWebSocketStore((state) => state.metar);
export const useArrAtisCode = () => useWebSocketStore((state) => state.arrAtisCode);
export const useDepAtisCode = () => useWebSocketStore((state) => state.depAtisCode);
/** @deprecated use useDepAtisCode or useArrAtisCode */
export const useAtisCode = () => useWebSocketStore((state) => state.depAtisCode);

export const useAvailableSids = () => useWebSocketStore((state) => state.availableSids);
export const useInitialCflByRunway = () => useWebSocketStore((state) => state.initialCflByRunway);
export const useTransitionAltitude = () => useWebSocketStore((state) => state.transitionAltitude);

export const useLowerPositionOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some(
      (c) => c.position !== state.position && (controllerOwnsDeliverySector(c) || controllerOwnsApronSector(c))
    )
  );

export const useDelOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some(
      (c) => c.position !== state.position && controllerOwnsDeliverySector(c)
    )
  );

export const useApronOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some(
      (c) => c.position !== state.position && controllerOwnsApronSector(c)
    )
  );

export const useCtwrOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some(
      (c) => c.position === "118.580" && c.position !== state.position
    )
  );

export const useTwrOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some(
      (c) => c.position !== state.position && controllerOwnsTowerSectors(c)
    )
  );

export const useIsTwr = () =>
  useWebSocketStore((state) =>
    controllerOwnsTowerSectors(state.controllers.find((c) => c.position === state.position))
  );

export const useIsClrDel = () =>
  useWebSocketStore((state) =>
    state.controllers.find((c) => c.position === state.position)?.section === "DEL"
  );

export const useLayoutChooserOpen = () => useWebSocketStore((state) => state.layoutChooserOpen);
export const useSetLayoutChooserOpen = () => useWebSocketStore((state) => state.setLayoutChooserOpen);
export const useContextMenu = () => useWebSocketStore((state) => state.contextMenu);
export const useOpenStripContextMenu = () => useWebSocketStore((state) => state.openStripContextMenu);
export const useCloseStripContextMenu = () => useWebSocketStore((state) => state.closeStripContextMenu);
