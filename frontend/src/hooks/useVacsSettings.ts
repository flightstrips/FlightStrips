import { useCallback, useEffect, useState, startTransition } from "react";
import {
  isVacsIntegrationEnabled,
  setVacsIntegrationEnabled,
} from "@/lib/vacs-settings";

export function useVacsSettings() {
  const [enabled, setEnabled] = useState(isVacsIntegrationEnabled);

  useEffect(() => {
    const onChange = (event: Event) => {
      const detail = (event as CustomEvent<boolean>).detail;
      setEnabled(typeof detail === "boolean" ? detail : isVacsIntegrationEnabled());
    };
    window.addEventListener("vacs-settings-changed", onChange);
    return () => window.removeEventListener("vacs-settings-changed", onChange);
  }, []);

  const setVacsEnabled = useCallback((value: boolean) => {
    setVacsIntegrationEnabled(value);
    startTransition(() => {
      setEnabled(value);
    });
    window.dispatchEvent(new CustomEvent("vacs-settings-changed", { detail: value }));
  }, []);

  return { vacsEnabled: enabled, setVacsEnabled };
}
