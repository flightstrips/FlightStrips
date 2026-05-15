import { useEffect, useState } from "react";
import { Settings } from "lucide-react";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { Button } from "@/components/ui/button";
import { useAudioSettings } from "@/hooks/useAudioSettings";
import { useVacsSettings } from "@/hooks/useVacsSettings";

const CLS_DIALOG = "sm:max-w-[400px] bg-[#b3b3b3]";

export default function CommandBarSettings() {
  const { muted, toggleMute } = useAudioSettings();
  const { vacsEnabled, setVacsEnabled, vacsHost, setVacsHost } = useVacsSettings();
  const [hostDraft, setHostDraft] = useState(vacsHost);

  useEffect(() => {
    setHostDraft(vacsHost);
  }, [vacsHost]);

  const commitHost = () => {
    if (hostDraft !== vacsHost) {
      setVacsHost(hostDraft);
    }
  };

  return (
    <Dialog
      onOpenChange={(open) => {
        if (open) {
          setHostDraft(vacsHost);
        }
      }}
    >
      <DialogTrigger asChild>
        <button
          type="button"
          className={`h-[3.42dvh] my-[0.65dvh] w-[3.52vw] flex items-center justify-center shadow-[inset_2px_0_0_var(--color-bay-shadow),_inset_0_2px_0_var(--color-bay-shadow)] outline-none ${
            muted ? "bg-[#FF4444] text-white" : "bg-bay-btn text-white"
          }`}
          aria-label="Settings"
        >
          <Settings className="w-[1.6vw] h-[1.6vw]" />
        </button>
      </DialogTrigger>
      <DialogContent className={CLS_DIALOG}>
        <VisuallyHidden.Root>
          <DialogTitle>Settings</DialogTitle>
        </VisuallyHidden.Root>
        <div className="border-2 border-black">
          <div className="p-4 space-y-4 min-w-[280px]">
            <h2 className="text-lg font-bold text-black">Settings</h2>

            <label className="flex items-center gap-3 cursor-pointer text-sm text-black">
              <input
                type="checkbox"
                checked={muted}
                onChange={() => toggleMute()}
                className="size-4"
              />
              <span>Mute strip sounds</span>
            </label>

            <div className="space-y-2">
              <label className="flex items-center gap-3 cursor-pointer text-sm text-black">
                <input
                  type="checkbox"
                  checked={vacsEnabled}
                  onChange={(e) => setVacsEnabled(e.target.checked)}
                  className="size-4"
                />
                <span>Enable VACS voice integration</span>
              </label>

              <label className="block pl-7 space-y-1 text-sm text-black">
                <span className="text-xs text-gray-800">VACS machine address (optional)</span>
                <input
                  type="text"
                  value={hostDraft}
                  onChange={(e) => setHostDraft(e.target.value)}
                  onBlur={commitHost}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      commitHost();
                    }
                  }}
                  placeholder="192.168.1.10"
                  className="w-full px-2 py-1 text-sm border border-black bg-white text-black"
                  aria-label="VACS machine address"
                />
                <span className="text-xs text-gray-600 block">
                  Leave empty to use this machine (localhost).
                </span>
              </label>
            </div>
          </div>
          <DialogFooter className="flex justify-center w-full h-14">
            <DialogClose asChild>
              <Button variant="darkaction" className="w-4/5">
                ESC
              </Button>
            </DialogClose>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  );
}
