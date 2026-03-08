import { cn } from "@/lib/utils";

interface DashedLineProps {
  orientation?: "horizontal" | "vertical";
  className?: string;
}

export function DashedLine({
  orientation = "horizontal",
  className,
}: DashedLineProps) {
  const isHorizontal = orientation === "horizontal";

  return (
    <div
      className={cn(
        "border-border border-dashed",
        isHorizontal
          ? "h-0 w-full border-t"
          : "h-full w-0 border-l min-h-8",
        className
      )}
    />
  );
}
