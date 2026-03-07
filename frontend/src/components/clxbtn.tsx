import React from "react";

import FlightPlanDialog from "@/components/FlightPlanDialog";

export function CLXBtn({ callsign, children }: { callsign: string; children?: React.ReactNode }) {
  return (
    <FlightPlanDialog callsign={callsign}>
      <div className="px-0" style={{ flex: "1 0 0%", height: "100%", minWidth: 0 }}>
        {children}
      </div>
    </FlightPlanDialog>
  );
}
