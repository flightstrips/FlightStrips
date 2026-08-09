export type StripStatus = "PENDING" | "ACTIVE" | "COMPLETED";

export interface Strip {
  id: string;
  callsign: string;
  departure: string;
  destination: string;
  stand: string;
  status: StripStatus;
  updatedAt: string;
}

export async function fetchStrips(): Promise<Strip[]> {
  const response = await fetch("/api/strips");
  if (!response.ok) {
    throw new Error("Failed to fetch strips");
  }
  return response.json();
}

export async function updateStripStatus(
  id: string,
  status: StripStatus,
): Promise<Strip> {
  const response = await fetch(`/api/strips/${id}/status`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ status }),
  });

  if (!response.ok) {
    throw new Error("Failed to update strip status");
  }

  return response.json();
}
