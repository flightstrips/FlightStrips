import { useState, useCallback } from "react";
import { isAudioMuted, setAudioMuted } from "@/lib/audio-settings";

export function useAudioSettings() {
  const [muted, setMuted] = useState<boolean>(isAudioMuted);

  const toggleMute = useCallback(() => {
    setMuted((prev) => {
      const next = !prev;
      setAudioMuted(next);
      return next;
    });
  }, []);

  return { muted, toggleMute };
}
