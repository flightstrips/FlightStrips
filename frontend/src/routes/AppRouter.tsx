import { useWebSocketStore } from "@/store/store-hooks";
import EKCHDEL from "@/routes/ekch/CLX";
import EKCHAAAD from "@/routes/ekch/AAAD";
import EKCHGEGW from "@/routes/ekch/GEGW";
import EKCHTWTE from "@/routes/ekch/TWTE";

const LAYOUT_MAP: Record<string, React.ComponentType> = {
  CLX: EKCHDEL,
  AAAD: EKCHAAAD,
  GEGW: EKCHGEGW,
  TWTE: EKCHTWTE,
};

export default function AppRouter() {
  const displayedLayout = useWebSocketStore((s) => s.displayedLayout);
  const Component = LAYOUT_MAP[displayedLayout];

  if (!Component) {
    return (
      <div className="w-screen min-h-svh flex justify-center items-center bg-primary text-white text-xl">
        Unknown layout: {displayedLayout}
      </div>
    );
  }

  return <Component />;
}
