import { Button } from "@/components/ui/button";
import { useWebSocketStore } from "@/store/store-hooks";

const EKCH_SCOPES = [
  { label: "CLR DEL", layout: "CLX" },
  { label: "AA + AD", layout: "AAAD" },
  { label: "APRON ARR", layout: "AA" },
  { label: "APRON DEP", layout: "AD" },
  { label: "EST", layout: "EST" },
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
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-primary">
      <div className="bg-[#b3b3b3] border border-border shadow-lg w-[300px]">
        <div className="border-2 border-black">
          <div className="flex justify-center w-full h-14">
            <Button variant="darkaction" className="w-full rounded-none" disabled>
              SELECT VIEW
            </Button>
          </div>
          <div className="grid grid-cols-2 gap-2 p-2">
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
    </div>
  );
}
