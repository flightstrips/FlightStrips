const AUDIO_MUTED_KEY = "audio-muted";

export function isAudioMuted(): boolean {
  if (typeof window === "undefined") return false;
  return localStorage.getItem(AUDIO_MUTED_KEY) === "true";
}

export function setAudioMuted(muted: boolean): void {
  localStorage.setItem(AUDIO_MUTED_KEY, String(muted));
}
