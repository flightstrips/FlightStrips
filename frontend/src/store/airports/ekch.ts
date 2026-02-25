import {useMemo} from "react";
import {useAirport, useWebSocketStore} from "@/store/store-hooks.ts";
import {Bay, type FrontendStrip} from "@/api/models.ts";

const isSasStrip = (strip: FrontendStrip) =>
  strip.callsign.toUpperCase().startsWith("SAS");
const isNorwegianStrip = (strip: FrontendStrip) =>
  strip.callsign.toUpperCase().startsWith("NSZ");

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

export const usePushbackStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  return useMemo(
    () => strips.filter(x => x.bay === Bay.Push).sort((a, b) => a.sequence - b.sequence),
    [strips]
  );
};

export const useTaxiDepStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  const airport = useAirport();
  return useMemo(
    () =>
      strips
        .filter(x => x.bay === Bay.Taxi && x.origin === airport)
        .sort((a, b) => a.sequence - b.sequence),
    [strips, airport]
  );
};

export const useTaxiArrStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  const airport = useAirport();
  return useMemo(
    () =>
      strips
        .filter(x => x.bay === Bay.Taxi && x.destination === airport)
        .sort((a, b) => a.sequence - b.sequence),
    [strips, airport]
  );
};

export const useDepartStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  return useMemo(
    () => strips.filter(x => x.bay === Bay.Depart).sort((a, b) => a.sequence - b.sequence),
    [strips]
  );
};

export const useAirborneStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  return useMemo(
    () => strips.filter(x => x.bay === Bay.Airborne).sort((a, b) => a.sequence - b.sequence),
    [strips]
  );
};

export const useFinalStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  return useMemo(
    () => strips.filter(x => x.bay === Bay.Final).sort((a, b) => a.sequence - b.sequence),
    [strips]
  );
};

export const useRwyArrStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  const airport = useAirport();
  return useMemo(
    () =>
      strips
        .filter(x => x.bay === Bay.Final && x.destination === airport)
        .sort((a, b) => a.sequence - b.sequence),
    [strips, airport]
  );
};

export const useStandStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  return useMemo(
    () => strips.filter(x => x.bay === Bay.Stand).sort((a, b) => a.sequence - b.sequence),
    [strips]
  );
};

export const useHiddenStrips = () => {
  const strips = useWebSocketStore(state => state.strips);
  return useMemo(
    () => strips.filter(x => x.bay === Bay.Hidden).sort((a, b) => a.sequence - b.sequence),
    [strips]
  );
};
