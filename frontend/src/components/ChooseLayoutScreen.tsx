import { Button } from "@/components/ui/button";
import { useWebSocketStore } from "@/store/store-hooks";

const EKCH_SCOPES = [
  { label: "CLR DEL", layout: "CLX" },
  { label: "AA + AD", layout: "AAAD" },
  { label: "ESET", layout: "ESET" },
  { label: "GE / GW", layout: "GEGW" },
  { label: "TW / TE", layout: "TWTE" },
];

/**
 * Full-screen overlay that forces the controller to choose a layout.
 * It is non-dismissable: no ESC key, no click-outside, no close button.
 */
export default function ChooseLayoutScreen() {
  const setDisplayedLayout = useWebSocketStore((s) => s.setDisplayedLayout);
  const setLayoutChooserOpen = useWebSocketStore((s) => s.setLayoutChooserOpen);

  function handleSelect(layout: string) {
    setDisplayedLayout(layout);
    setLayoutChooserOpen(false);
  }

  return (
    <div className="w-screen h-screen fixed inset-0 z-50 flex items-center justify-center bg-primary">
      <div className="bg-[#b3b3b3] p-6 border-2 border-black w-72">
        <h2 className="text-black font-bold text-lg mb-4 text-center">SELECT VIEW</h2>
        <div className="grid grid-cols-2 gap-2">
          {EKCH_SCOPES.map((scope) => (
            <Button
              key={scope.layout}
              variant="trf"
              className="font-normal text-base h-fit py-3"
              onClick={() => handleSelect(scope.layout)}
            >
              {scope.label}
            </Button>
          ))}
        </div>
      </div>
    </div>
  );
}
