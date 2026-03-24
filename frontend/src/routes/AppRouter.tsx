import { useWebSocketStore } from "@/store/store-hooks";
import EKCHDEL from "@/routes/ekch/CLX";
import EKCHAAAD from "@/routes/ekch/AAAD";
import EKCHAA from "@/routes/ekch/AA";
import EKCHAAD from "@/routes/ekch/AD";
import EKCHESET from "@/routes/ekch/ESET";
import EKCHGEGW from "@/routes/ekch/GEGW";
import EKCHTWTE from "@/routes/ekch/TWTE";
import ChooseLayoutScreen from "@/components/ChooseLayoutScreen";

const LAYOUT_MAP: Record<string, React.ComponentType> = {
  CLX: EKCHDEL,
  AAAD: EKCHAAAD,
  AA: EKCHAA,
  AD: EKCHAAD,
  ESET: EKCHESET,
  GEGW: EKCHGEGW,
  TWTE: EKCHTWTE,
};

export default function AppRouter() {
  const displayedLayout = useWebSocketStore((s) => s.displayedLayout);
  const Component = LAYOUT_MAP[displayedLayout];

  if (!Component) {
    return <ChooseLayoutScreen />;
  }

  return <Component />;
}
