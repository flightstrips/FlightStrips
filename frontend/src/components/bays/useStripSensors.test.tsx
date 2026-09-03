import { DndContext, useDraggable, type DragMoveEvent } from "@dnd-kit/core";
import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { useStripSensors } from "./useStripSensors";

function TouchDragHarness({ onDragMove }: { onDragMove: (event: DragMoveEvent) => void }) {
  const sensors = useStripSensors();
  const [dragStarted, setDragStarted] = useState(false);

  return (
    <DndContext sensors={sensors} onDragStart={() => setDragStarted(true)} onDragMove={onDragMove}>
      <Draggable />
      <output>{dragStarted ? "started" : "idle"}</output>
    </DndContext>
  );
}

function Draggable() {
  const { attributes, listeners, setNodeRef } = useDraggable({ id: "strip" });
  return (
    <div ref={setNodeRef} {...attributes} {...listeners}>
      Strip
    </div>
  );
}

function touchEvent(type: string, clientX: number, clientY: number) {
  const event = new Event(type, {
    bubbles: true,
    cancelable: true,
  });
  const touch = { clientX, clientY };
  Object.defineProperties(event, {
    clientX: { value: clientX },
    clientY: { value: clientY },
  });
  Object.defineProperty(event, "touches", { value: type === "touchend" ? [] : [touch] });
  Object.defineProperty(event, "changedTouches", { value: [touch] });
  return event;
}

describe("strip touch sensor", () => {
  it("keeps touch taps idle and starts dragging on the first pointer movement", () => {
    const onDragMove = vi.fn();
    render(<TouchDragHarness onDragMove={onDragMove} />);
    const strip = screen.getByText("Strip");

    fireEvent(strip, touchEvent("touchstart", 10, 10));
    expect(screen.getByText("idle")).toBeInTheDocument();

    const dragMove = touchEvent("touchmove", 30, 20);
    fireEvent(strip, dragMove);
    expect(screen.getByText("started")).toBeInTheDocument();

    expect(dragMove.defaultPrevented).toBe(true);
    expect(onDragMove).toHaveBeenCalled();
  });

  it("leaves an immediate swipe available for scrolling in an overflowing bay", () => {
    const onDragMove = vi.fn();
    render(
      <div data-strip-scroll-container="true" data-testid="scroll-container">
        <TouchDragHarness onDragMove={onDragMove} />
      </div>,
    );
    const container = screen.getByTestId("scroll-container");
    Object.defineProperties(container, {
      clientHeight: { value: 100 },
      scrollHeight: { value: 200 },
    });
    const strip = screen.getByText("Strip");

    fireEvent(strip, touchEvent("touchstart", 10, 50));
    const scrollMove = touchEvent("touchmove", 10, 20);
    fireEvent(strip, scrollMove);

    expect(screen.getByText("idle")).toBeInTheDocument();
    expect(scrollMove.defaultPrevented).toBe(false);
    expect(onDragMove).not.toHaveBeenCalled();
  });
});
