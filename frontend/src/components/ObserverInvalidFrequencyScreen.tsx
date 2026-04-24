import { usePosition } from "@/store/store-hooks";

export default function ObserverInvalidFrequencyScreen() {
  const position = usePosition();

  return (
    <div className="min-h-svh bg-primary text-white flex items-center justify-center">
      <div className="max-w-3xl px-8 text-center">
        <div className="text-sm font-semibold tracking-[0.25em] text-[#FFD84D]">
          OBSERVER MODE
        </div>
        <h1 className="mt-4 text-5xl font-semibold">
          Invalid observer frequency
        </h1>
        <p className="mt-6 text-xl text-white/85">
          Your primary frequency{position ? ` (${position})` : ""} does not match any online controller.
        </p>
        <p className="mt-3 text-lg text-white/70">
          Choose a primary frequency in EuroScope that is currently used by an online controller to observe that position.
        </p>
      </div>
    </div>
  );
}
