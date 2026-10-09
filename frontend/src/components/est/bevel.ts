export const EST_RAISED_EDGE = {
  border: "1px solid",
  borderColor: "#CECECE #393939 #393939 #CECECE",
  boxShadow: "inset 1px 1px 1px rgba(206,206,206,0.55), inset -1px -1px 1px rgba(57,57,57,0.55)",
  boxSizing: "border-box" as const,
};

export const EST_SUNKEN_EDGE = {
  border: "1px solid",
  borderColor: "#393939 #CECECE #CECECE #393939",
  boxShadow: "inset 1px 1px 1px rgba(57,57,57,0.55), inset -1px -1px 1px rgba(206,206,206,0.55)",
  boxSizing: "border-box" as const,
};

export const EST_RAISED_BUTTON_EDGE = {
  boxShadow: EST_RAISED_EDGE.boxShadow,
};
