import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import type { FrontendController } from "@/api/models";
import { useControllers, useMyPosition } from "@/store/store-hooks";
import { useVacs } from "@/hooks/useVacs";
import { findVacsClientForController } from "@/vacs/match";
import type { ClientInfo } from "@/vacs/types";

const CLS_DIALOG = "sm:max-w-[480px] bg-[#b3b3b3]";

interface VacsDialModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  clients: ClientInfo[];
  ownClientId: string | null;
  ownPositionId: string;
  ambiguous: boolean;
}

export default function VacsDialModal({
  open,
  onOpenChange,
  clients,
  ownClientId,
  ownPositionId,
  ambiguous,
}: VacsDialModalProps) {
  const [filter, setFilter] = useState("");
  const controllers = useControllers();
  const myPosition = useMyPosition();
  const { actions } = useVacs();

  const peers = useMemo(() => {
    const others = controllers.filter((c) => c.position !== myPosition);
    const available: Array<{ controller: FrontendController; vacs: ClientInfo }> = [];
    const notOnVacs: FrontendController[] = [];

    const dialableClients = clients.filter((c) => c.id !== ownClientId);

    for (const controller of others) {
      const vacs = findVacsClientForController(controller, dialableClients);
      if (vacs) {
        available.push({ controller, vacs });
      } else {
        notOnVacs.push(controller);
      }
    }

    const q = filter.trim().toLowerCase();
    const matches = (c: FrontendController) =>
      !q ||
      c.callsign.toLowerCase().includes(q) ||
      c.position.toLowerCase().includes(q);

    return {
      available: available.filter((e) => matches(e.controller)),
      notOnVacs: notOnVacs.filter(matches),
    };
  }, [clients, controllers, filter, myPosition, ownClientId]);

  const canDial = !!ownPositionId && !ambiguous;

  const handleDial = async (entry: { controller: FrontendController; vacs: ClientInfo }) => {
    try {
      await actions.dialClient(entry.vacs);
      onOpenChange(false);
    } catch {
      // placeCall already surfaces a toast
    }
  };

  const showSearch =
    controllers.filter((c) => c.position !== myPosition).length > 10;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={CLS_DIALOG}>
        <VisuallyHidden.Root>
          <DialogTitle>Voice — dial controller</DialogTitle>
        </VisuallyHidden.Root>
        <div className="border-2 border-black">
          <div className="p-2 space-y-3 max-h-[50vh] overflow-y-auto">
            {showSearch && (
              <input
                type="text"
                placeholder="Search callsign or position…"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                className="w-full px-2 py-1 text-sm border border-black bg-white text-black"
              />
            )}
            {peers.available.length === 0 && peers.notOnVacs.length === 0 ? (
              <p className="text-center text-sm text-gray-700 py-4">
                No other controllers currently on VACS.
              </p>
            ) : (
              <>
                {peers.available.length > 0 && (
                  <section>
                    <h3 className="text-xs font-bold text-gray-800 mb-1">Available</h3>
                    <ul className="space-y-1">
                      {peers.available.map(({ controller, vacs }) => (
                        <li
                          key={controller.callsign}
                          className="flex items-center justify-between gap-2 bg-white/60 px-2 py-1 text-sm"
                        >
                          <div>
                            <span className="font-semibold">{controller.callsign}</span>
                            <span className="text-gray-600 ml-2">{vacs.frequency}</span>
                          </div>
                          <Button
                            variant="trf"
                            className="h-7 text-xs px-2"
                            disabled={!canDial}
                            title={
                              !ownPositionId
                                ? "VACS is not connected to a signaling position."
                                : ambiguous
                                  ? "Your VACS position is ambiguous — resolve it in VACS."
                                  : undefined
                            }
                            onClick={() => handleDial({ controller, vacs })}
                          >
                            Dial
                          </Button>
                        </li>
                      ))}
                    </ul>
                  </section>
                )}
                {peers.notOnVacs.length > 0 && (
                  <section>
                    <h3 className="text-xs font-bold text-gray-500 mb-1">Not on VACS</h3>
                    <ul className="space-y-1 opacity-60">
                      {peers.notOnVacs.map((controller) => (
                        <li
                          key={controller.callsign}
                          className="flex items-center justify-between gap-2 px-2 py-1 text-sm text-gray-600"
                        >
                          <span>{controller.callsign}</span>
                          <span className="text-xs italic">not on VACS</span>
                        </li>
                      ))}
                    </ul>
                  </section>
                )}
              </>
            )}
          </div>
          <DialogFooter className="flex justify-center w-full h-14 border-t border-black">
            <Button variant="darkaction" className="w-4/5" onClick={() => onOpenChange(false)}>
              ESC
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  );
}
