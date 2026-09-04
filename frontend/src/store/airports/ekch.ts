import {useMemo} from "react";
import {useActiveMessages, useTacticalStrips, useWebSocketStore} from "@/store/store-hooks.ts";
import {Bay, type FrontendStrip, type TacticalStrip, type AnyStrip, isFlight} from "@/api/models.ts";

export type { AnyStrip };
export { isFlight };

export const selectFlightStripsForBay = (strips: FrontendStrip[], bay: Bay) =>
  strips.filter((strip) => strip.bay === bay);

export const selectStripsForBay = (strips: FrontendStrip[], tacticalStrips: TacticalStrip[], bay: Bay): AnyStrip[] =>
  [
    ...selectFlightStripsForBay(strips, bay),
    ...tacticalStrips.filter((strip) => strip.bay === bay),
  ].sort((a, b) => a.sequence - b.sequence);

export const selectHiddenFlightStrips = (strips: FrontendStrip[]) =>
  strips.filter((strip) => strip.bay === Bay.Hidden || strip.bay === Bay.HiddenDep);

export const useTacticalStripsForBay = (bay: Bay) => {
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => tacticalStrips.filter(t => t.bay === bay).sort((a, b) => a.sequence - b.sequence),
    [tacticalStrips, bay]
  );
};

const SAS_PREFIXES = ["SAS", "SZS"];
const NORWEGIAN_PREFIXES = ["NAX", "NOZ", "NSZ", "IBK"];

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
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => selectStripsForBay(strips, tacticalStrips, Bay.Cleared),
    [strips, tacticalStrips]
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
  return useMemo(
    () => ([
      ...selectFlightStripsForBay(strips, Bay.Taxi),
      ...tacticalStrips.filter(t => t.bay === Bay.Taxi),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
  );
};

export const useTaxiDepLwrStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => ([
      ...selectFlightStripsForBay(strips, Bay.TaxiLwr),
      ...tacticalStrips.filter(t => t.bay === Bay.TaxiLwr),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
  );
};

export const useTaxiArrStrips = (): AnyStrip[] => {
  const strips = useWebSocketStore(state => state.strips);
  const tacticalStrips = useTacticalStrips();
  return useMemo(
    () => ([
      ...selectFlightStripsForBay(strips, Bay.TwyArr),
      ...tacticalStrips.filter(t => t.bay === Bay.TwyArr),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
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
  return useMemo(
    () => ([
      ...selectFlightStripsForBay(strips, Bay.RwyArr),
      ...tacticalStrips.filter(t => t.bay === Bay.RwyArr),
    ] as AnyStrip[]).sort((a, b) => a.sequence - b.sequence),
    [strips, tacticalStrips]
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
      ...selectHiddenFlightStrips(strips),
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

