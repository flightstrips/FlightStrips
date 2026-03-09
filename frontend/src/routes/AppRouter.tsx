import { useEffect } from "react";
import { useWebSocketStore } from "@/store/store-hooks";
import EKCHDEL from "@/routes/ekch/CLX";
import EKCHAAAD from "@/routes/ekch/AAAD";
import EKCHESET from "@/routes/ekch/ESET";
import EKCHGEGW from "@/routes/ekch/GEGW";
import EKCHTWTE from "@/routes/ekch/TWTE";

const LAYOUT_MAP: Record<string, React.ComponentType> = {
  CLX: EKCHDEL,
  AAAD: EKCHAAAD,
  ESET: EKCHESET,
  GEGW: EKCHGEGW,
  TWTE: EKCHTWTE,
};

export default function AppRouter() {
  const displayedLayout = useWebSocketStore((s) => s.displayedLayout);
  const setLayoutChooserOpen = useWebSocketStore((s) => s.setLayoutChooserOpen);
  const Component = LAYOUT_MAP[displayedLayout];

  useEffect(() => {
    if (!Component) {
      setLayoutChooserOpen(true);
    }
  }, [Component, setLayoutChooserOpen]);

  if (!Component) {
    return (
      <div className="w-screen min-h-svh bg-primary" />
    );
  }

  return <Component />;
}
