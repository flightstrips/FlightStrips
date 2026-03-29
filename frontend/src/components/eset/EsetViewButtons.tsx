import {
  ESET_VIEW_BUTTONS,
  type EsetView,
  type EsetViewButtonId,
} from "@/components/eset/metadata";

interface EsetViewButtonsProps {
  view: EsetView;
  onViewChange: (view: EsetView) => void;
}

function getNextView(buttonId: EsetViewButtonId, currentView: EsetView) {
  if (buttonId === "CARGO") {
    return currentView === "CARGO" ? "MAIN" : "CARGO";
  }

  return currentView;
}

export default function EsetViewButtons({ view, onViewChange }: EsetViewButtonsProps) {
  return (
    <>
      {ESET_VIEW_BUTTONS.map((button) => {
        const active = button.id === "CARGO" && view === "CARGO";
        const disabled = !!button.disabled;

        return (
          <button
            key={button.id}
            type="button"
            disabled={disabled}
            aria-pressed={active}
            onClick={() => {
              if (!disabled) {
                onViewChange(getNextView(button.id, view));
              }
            }}
            className="absolute z-10 flex items-center justify-center border border-black/20 font-bold shadow-[inset_0_2px_0_rgba(255,255,255,0.08)] outline-none transition-colors focus-visible:outline-2 focus-visible:outline-white"
            style={{
              left: button.x,
              top: button.y,
              width: button.width,
              height: button.height,
              borderRadius: button.radius ?? 0,
              backgroundColor: disabled ? "#2C2C2C" : active ? "#1BFF16" : button.fill,
              color: disabled ? "#9A9A9A" : active ? "#202020" : button.labelColor ?? "#FFFFFF",
              fontSize: 32,
              opacity: disabled ? 0.55 : 1,
              cursor: disabled ? "not-allowed" : "pointer",
            }}
          >
            {button.label}
          </button>
        );
      })}
    </>
  );
}
