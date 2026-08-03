export type ReplayErrorEvent = {
  type: "threshold_crossing" | "go_around_detected" | "landing_proxy";
  at: string;
  sequence?: number;
};

export type OperationalErrorReference = {
  at: string;
  kind: "first_touchdown" | "go_around_target" | "landing_proxy";
  goAroundDetectedAt?: string;
};

export function operationalErrorReference(
  events: ReplayErrorEvent[],
  predictionAt: string,
  landingProxyAt: string,
  goAroundDelaySeconds: number,
): OperationalErrorReference {
  const predictionTime = new Date(predictionAt).getTime();
  const latestGoAround = events
    .filter((event) => event.type === "go_around_detected" && new Date(event.at).getTime() <= predictionTime)
    .at(-1);

  if (latestGoAround) {
    return {
      at: new Date(new Date(latestGoAround.at).getTime() + goAroundDelaySeconds * 1000).toISOString(),
      kind: "go_around_target",
      goAroundDetectedAt: latestGoAround.at,
    };
  }

  const firstThreshold = events.find((event) => event.type === "threshold_crossing" && event.sequence === 1)
    ?? events.find((event) => event.type === "threshold_crossing");
  if (firstThreshold) return {at: firstThreshold.at, kind: "first_touchdown"};

  return {at: landingProxyAt, kind: "landing_proxy"};
}
