import {useMemo} from "react";
import {useActiveMessages, useAirport, useTacticalStrips, useWebSocketStore} from "@/store/store-hooks.ts";
import {Bay, type FrontendStrip, type AnyStrip, isFlight} from "@/api/models.ts";

export type { AnyStrip };
export { isFlight };

export const useTacticalStripsForBay = (bay: Bay) => {
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => tacticalStrips.filter(t => t.bay === bay).sort((a, b) => a.sequence - b.sequence),
    [tacticalStrips, bay]
  );
};

const SAS_PREFIXES = ["SAS", "SZS"];
const NORWEGIAN_PREFIXES = ["NAX", "NOZ", "NZS", "IBK"];

const isSasStrip = (strip: FrontendStrip) => {
  const cs = strip.callsign.toUpperCase();
  return SAS_PREFIXES.some(p => cs.startsWith(p));
};
const isNorwegianStrip = (strip: FrontendStrip) => {
  const cs = strip.callsign.toUpperCase();
  return NORWEGIAN_PREFIXES.some(p => cs.startsWith(p));
};

export const useSasBayStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  return useMemo(
    () => strips.filter(x => x.bay === Bay.NotCleared && isSasStrip(x)),
    [strips]
  );
};

export const useNorwegianBayStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  return useMemo(
    () => strips.filter(x => x.bay === Bay.NotCleared && isNorwegianStrip(x)),
    [strips]
  );
};

export const useClearedStrips = () => {
  const strips = useWebSocketStore(state => state.strips)
  return useMemo(
    () => strips.filter(x => x.bay === Bay.Cleared),
    [strips]
  );
}

export const useOtherBayStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  return useMemo(
    () =>
      strips.filter(
        x =>
          x.bay === Bay.NotCleared &&
          !isSasStrip(x) &&
          !isNorwegianStrip(x)
      ),
    [strips]
  );
};

export const useNonClearedStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  return useMemo(
    () => strips.filter(x => x.bay === Bay.NotCleared),
    [strips]
  );
}

export const usePushbackStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.Push),
      ...tacticalStrips.filter(t => t.bay === Bay.Push),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
  );
};

export const useTaxiDepStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  const airport = useAirport();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.Taxi && x.origin === airport),
      ...tacticalStrips.filter(t => t.bay === Bay.Taxi),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips, airport]
  );
};

export const useTaxiDepLwrStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  const airport = useAirport();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.TaxiLwr && x.origin === airport),
      ...tacticalStrips.filter(t => t.bay === Bay.TaxiLwr),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips, airport]
  );
};

export const useTaxiArrStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  const airport = useAirport();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.TwyArr && x.destination === airport),
      ...tacticalStrips.filter(t => t.bay === Bay.TwyArr),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips, airport]
  );
};

export const useDepartStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.Depart),
      ...tacticalStrips.filter(t => t.bay === Bay.Depart),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
  );
};

export const useAirborneStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.Airborne),
      ...tacticalStrips.filter(t => t.bay === Bay.Airborne),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
  );
};

export const useFinalStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.Final),
      ...tacticalStrips.filter(t => t.bay === Bay.Final),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
  );
};

export const useRwyArrStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  const airport = useAirport();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.RwyArr && x.destination === airport),
      ...tacticalStrips.filter(t => t.bay === Bay.RwyArr),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips, airport]
  );
};

export const useStandStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.Stand),
      ...tacticalStrips.filter(t => t.bay === Bay.Stand),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
  );
};

export const useDeIceStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.DeIce),
      ...tacticalStrips.filter(t => t.bay === Bay.DeIce),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
  );
};

export const useControlzoneStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.Controlzone),
      ...tacticalStrips.filter(t => t.bay === Bay.Controlzone),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
  );
};

export const useHiddenStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => ([
      ...strips.filter(x => x.bay === Bay.Hidden),
      ...tacticalStrips.filter(t => t.bay === Bay.Hidden),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
  );
};

export const useInboundStrips = (): FrontendStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  return useMemo(
    () => strips.filter(x => x.bay === Bay.ArrHidden),
    [strips]
  );
};

export { useActiveMessages };

