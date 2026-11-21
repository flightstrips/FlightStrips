import type {ReactNode} from "react";
import {DialogContent, DialogTitle} from "./dialog";
import {cn} from "@/lib/utils";

export function FlightStripsPanel({children, className}: {children: ReactNode; className?: string}) {
  return <div className={cn("border border-bay-header bg-bay-header-light p-2 text-black shadow-[0_4px_4px_#0004]", className)}>
    <div className="grid min-w-0 gap-3 border border-bay-header p-4 [&_button]:rounded-none [&_button]:shadow-[0_2px_3px_#0004]">
      {children}
    </div>
  </div>;
}

export function FlightStripsDialogContent({title, children, className}: {
  title: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return <DialogContent className={cn("block max-h-[90svh] w-[min(460px,95vw)] overflow-y-auto border-0 bg-transparent p-0 shadow-none [&>button.absolute]:hidden", className)}>
    <FlightStripsPanel>
      <DialogTitle className="font-semibold">{title}</DialogTitle>
      {children}
    </FlightStripsPanel>
  </DialogContent>;
}
