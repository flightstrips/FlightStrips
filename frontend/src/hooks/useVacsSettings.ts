import { useCallback, useEffect, useState, startTransition } from "react";
import {
  isVacsIntegrationEnabled,
  setVacsIntegrationEnabled,
} from "@/lib/vacs-settings";

function notifyVacsSettingsChanged(): void {
  window.dispatchEvent(new CustomEvent("vacs-settings-changed"));
}

export function useVacsSettings() {
  const [enabled, setEnabled] = useState(isVacsIntegrationEnabled);

  useEffect(() => {
    const onChange = () => {
      setEnabled(isVacsIntegrationEnabled());
    };
    window.addEventListener("vacs-settings-changed", onChange);
    return () => window.removeEventListener("vacs-settings-changed", onChange);
  }, []);

  const setVacsEnabled = useCallback((value: boolean) => {
    setVacsIntegrationEnabled(value);
    startTransition(() => {
      setEnabled(value);
    });
    notifyVacsSettingsChanged();
  }, []);

  return {
    vacsEnabled: enabled,
    setVacsEnabled,
  };
}
