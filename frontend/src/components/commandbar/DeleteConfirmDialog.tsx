// Sizes derived from SVG canvas (2560×1440 base):
//   horizontal → / 2560 * 100 = vw
//   vertical   → / 1440 * 100 = vh
//
// Card: 380×302px → 14.84vw × 20.97vh
// Text font: 20px → 0.78vw  |  Button font: 32px → 1.25vw
// Button: 125×70px → 4.88vw × 4.86vh
// Yes left offset: 48px → 1.875vw  |  No left offset: 213px → 8.32vw (relative to card)
// Button top: 173/1440*100 → 12.01vh absolute, but ~8.24vh from card top (173/302*20.97)
// Gap between buttons: 40px → 1.56vw

interface Props {
  onConfirm: () => void;
  onCancel: () => void;
}

export default function DeleteConfirmDialog({ onConfirm, onCancel }: Props) {
  return (
    <>
      {/* Backdrop */}
      <div className="fixed inset-0 z-40" onClick={onCancel} />

      {/* Centered card */}
      <div
        className="fixed z-50 flex items-center justify-center"
        style={{ inset: 0, pointerEvents: "none" }}
      >
        <div
          style={{
            width: "14.84vw",
            height: "20.97vh",
            background: "#B3B3B3",
            border: "1px solid black",
            position: "relative",
            pointerEvents: "auto",
          }}
        >
          {/* Text block — vertically centered in upper portion */}
          <div
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              right: 0,
              // Bottom of text area ends where buttons start (~57% down)
              bottom: "42.97%",
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              justifyContent: "center",
              fontFamily: "Rubik, sans-serif",
              fontSize: "0.78vw",
              fontWeight: 600,
              color: "black",
              lineHeight: 1.3,
              textAlign: "center",
            }}
          >
            <span>Are you sure</span>
            <span>You want to delete this strip?</span>
          </div>

          {/* Buttons row */}
          <div
            style={{
              position: "absolute",
              bottom: "19.54%", // (302 - 173 - 70) / 302 ≈ 19.54% from bottom
              left: 0,
              right: 0,
              display: "flex",
              justifyContent: "center",
              gap: "1.56vw",
            }}
          >
            {/* Yes */}
            <button
              onClick={onConfirm}
              style={{
                width: "4.88vw",
                height: "4.86vh",
                background: "#D6D6D6",
                color: "black",
                fontFamily: "Rubik, sans-serif",
                fontSize: "1.25vw",
                fontWeight: 600,
                boxShadow: "0 4px 4px rgba(0,0,0,0.25)",
                border: "none",
                cursor: "pointer",
              }}
            >
              Yes
            </button>

            {/* No */}
            <button
              onClick={onCancel}
              style={{
                width: "4.88vw",
                height: "4.86vh",
                background: "#D6D6D6",
                color: "black",
                fontFamily: "Rubik, sans-serif",
                fontSize: "1.25vw",
                fontWeight: 600,
                boxShadow: "0 4px 4px rgba(0,0,0,0.25)",
                border: "none",
                cursor: "pointer",
              }}
            >
              No
            </button>
          </div>
        </div>
      </div>
    </>
  );
}
