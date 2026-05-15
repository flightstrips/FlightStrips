import { useCallback, useEffect, useState, startTransition } from "react";
import {
  getVacsHost,
  isVacsIntegrationEnabled,
  normalizeVacsHostInput,
  setVacsHost,
  setVacsIntegrationEnabled,
} from "@/lib/vacs-settings";

function notifyVacsSettingsChanged(): void {
  window.dispatchEvent(new CustomEvent("vacs-settings-changed"));
}

export function useVacsSettings() {
  const [enabled, setEnabled] = useState(isVacsIntegrationEnabled);
  const [host, setHost] = useState(getVacsHost);

  useEffect(() => {
    const onChange = () => {
      setEnabled(isVacsIntegrationEnabled());
      setHost(getVacsHost());
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

  const setVacsHostSetting = useCallback((value: string) => {
    const normalized = normalizeVacsHostInput(value);
    setVacsHost(normalized);
    startTransition(() => {
      setHost(normalized);
    });
    notifyVacsSettingsChanged();
  }, []);

  return {
    vacsEnabled: enabled,
    setVacsEnabled,
    vacsHost: host,
    setVacsHost: setVacsHostSetting,
  };
}
