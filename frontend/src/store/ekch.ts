import { useMemo } from "react";
import { useWebSocketStore } from "@/store/store-provider";
import { Bay, type FrontendStrip } from "@/api/models";

const isSasStrip = (strip: FrontendStrip) =>
  strip.callsign.toUpperCase().startsWith("SAS");
const isNorwegianStrip = (strip: FrontendStrip) =>
  strip.callsign.toUpperCase().startsWith("NSZ");

export const useSasBayStrips = () => {
  const strips = useWebSocketStore((state) => state.strips);
  return useMemo(
    () => strips.filter((x) => x.bay === Bay.NotCleared && isSasStrip(x)),
    [strips]
  );
};

export const useNorwegianBayStrips = () => {
  const strips = useWebSocketStore((state) => state.strips);
  return useMemo(
    () => strips.filter((x) => x.bay === Bay.NotCleared && isNorwegianStrip(x)),
    [strips]
  );
};

export const useClearedStrips = () => {
  const strips = useWebSocketStore((state) => state.strips);
  return useMemo(() => strips.filter((x) => x.bay === Bay.Cleared), [strips]);
};

export const useOtherBayStrips = () => {
  const strips = useWebSocketStore((state) => state.strips);
  return useMemo(
    () =>
      strips.filter(
        (x) =>
          x.bay === Bay.NotCleared && !isSasStrip(x) && !isNorwegianStrip(x)
      ),
    [strips]
  );
};
