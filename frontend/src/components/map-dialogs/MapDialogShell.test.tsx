import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MAP_POINT_STROKE_STYLE, MapDialogShell } from "./MapDialogShell";

describe("MapDialogShell restricted points", () => {
  it("disables restricted points and shows the map-specific tooltip on hover", () => {
    const onSelect = vi.fn();
    render(
      <MapDialogShell
        open
        onOpenChange={vi.fn()}
        title="Select point"
        imageSrc="/map.png"
        imageAlt="Map"
        imgWidth={100}
        imgHeight={100}
        points={[
          { label: "A", left: "25%", top: "25%" },
          { label: "B", left: "75%", top: "75%" },
        ]}
        pointStyle={MAP_POINT_STROKE_STYLE}
        btnStyle={{ width: "20px", height: "20px" }}
        onSelect={onSelect}
        isPointDisabled={label => label === "A"}
        disabledPointMessage="Aircraft type not approved for this Taxiway"
      />,
    );

    const blockedPoint = screen.getByRole("button", { name: "A" });
    expect(blockedPoint).toBeDisabled();
    expect(blockedPoint).toHaveStyle({ borderBottom: "1px solid #333", borderRight: "1px solid #333" });
    expect(blockedPoint.parentElement).not.toHaveAttribute("title");
    fireEvent.mouseEnter(blockedPoint);
    expect(screen.getByRole("tooltip")).toHaveTextContent("Aircraft type not approved for this Taxiway");
    fireEvent.click(blockedPoint);
    expect(onSelect).not.toHaveBeenCalled();

    const availablePoint = screen.getByRole("button", { name: "B" });
    expect(availablePoint).toBeEnabled();
    fireEvent.click(availablePoint);
    expect(onSelect).toHaveBeenCalledWith("B", { label: "B", left: "75%", top: "75%" });
  });

  it("renders static map labels as non-pressable buttons", () => {
    const onSelect = vi.fn();
    render(
      <MapDialogShell
        open
        onOpenChange={vi.fn()}
        title="Select point"
        imageSrc="/map.png"
        imageAlt="Map"
        imgWidth={100}
        imgHeight={100}
        points={[]}
        staticPoints={[{ label: "3001m", left: "40%", top: "70%" }]}
        btnStyle={{ width: "40px", height: "20px" }}
        onSelect={onSelect}
      />,
    );

    const toda = screen.getByRole("button", { name: "TODA 3001m" });
    expect(toda).toBeDisabled();
    expect(toda).toHaveStyle({ backgroundColor: "#938E8E" });
    expect(toda).toHaveStyle({ borderBottom: "1px solid #333", borderRight: "1px solid #333" });
    fireEvent.click(toda);
    expect(onSelect).not.toHaveBeenCalled();
  });
});
