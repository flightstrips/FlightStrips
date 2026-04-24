import { useWebSocketStore } from "@/store/store-hooks";
import EKCHDEL from "@/routes/ekch/CLX";
import EKCHAAAD from "@/routes/ekch/AAAD";
import EKCHAA from "@/routes/ekch/AA";
import EKCHAAD from "@/routes/ekch/AD";
import EKCHEST from "@/routes/ekch/EST";
import EKCHGEGW from "@/routes/ekch/GEGW";
import EKCHTWTE from "@/routes/ekch/TWTE";
import ChooseLayoutScreen from "@/components/ChooseLayoutScreen";
import ObserverInvalidFrequencyScreen from "@/components/ObserverInvalidFrequencyScreen";

const LAYOUT_MAP: Record<string, React.ComponentType> = {
  CLX: EKCHDEL,
  AAAD: EKCHAAAD,
  AA: EKCHAA,
  AD: EKCHAAD,
  EST: EKCHEST,
  GEGW: EKCHGEGW,
  TWTE: EKCHTWTE,
};

export default function AppRouter() {
  const displayedLayout = useWebSocketStore((s) => s.displayedLayout);
  const readOnly = useWebSocketStore((s) => s.readOnly);
  const positionAvailable = useWebSocketStore((s) => s.positionAvailable);
  const Component = LAYOUT_MAP[displayedLayout];

  if (!Component) {
    if (readOnly && !positionAvailable) {
      return <ObserverInvalidFrequencyScreen />;
    }
    return <ChooseLayoutScreen />;
  }

  return <Component />;
}
