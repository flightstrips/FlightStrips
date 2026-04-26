import { useEffect } from "react";
import { toast } from "sonner";
import { reloadForAppUpdate, subscribeToAppUpdates } from "@/lib/app-update";

const UPDATE_TOAST_ID = "flightstrips-app-update";

export default function AppUpdateNotifier() {
  useEffect(() => {
    const unsubscribe = subscribeToAppUpdates((state) => {
      if (!state.available) {
        return;
      }

      toast.info("A new FlightStrips update is available.", {
        id: UPDATE_TOAST_ID,
        description: "Reload to switch to the latest version.",
        duration: Infinity,
        action: {
          label: "Reload",
          onClick: () => {
            void reloadForAppUpdate();
          },
        },
      });
    });

    return () => {
      unsubscribe();
      toast.dismiss(UPDATE_TOAST_ID);
    };
  }, []);

  return null;
}
