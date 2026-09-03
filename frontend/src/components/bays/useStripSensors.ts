import { MouseSensor, TouchSensor, useSensor, useSensors } from "@dnd-kit/core";

type StripTouchSensorProps = ConstructorParameters<typeof TouchSensor>[0];

const MOUSE_DRAG_DISTANCE = 5;
const TOUCH_DRAG_DELAY_MS = 150;
const TOUCH_DRAG_TOLERANCE_PX = 8;
const STRIP_SCROLL_CONTAINER_SELECTOR = '[data-strip-scroll-container="true"]';

function canScrollVertically(element: HTMLElement | null) {
  return element != null && element.scrollHeight > element.clientHeight + 1;
}

function getScrollContainer(activeNode: StripTouchSensorProps["activeNode"]) {
  const activatorNode = activeNode.activatorNode.current ?? activeNode.node.current;
  return activatorNode?.closest<HTMLElement>(STRIP_SCROLL_CONTAINER_SELECTOR) ?? null;
}

interface TouchSensorInternals {
  autoScrollEnabled: boolean;
  activated: boolean;
  handleMove(event: Event): void;
}

interface TouchSensorConstructor {
  new (...args: ConstructorParameters<typeof TouchSensor>): TouchSensorInternals;
  activators: typeof TouchSensor.activators;
  setup?: typeof TouchSensor.setup;
}

const TouchSensorWithInternals = TouchSensor as unknown as TouchSensorConstructor;

/**
 * dnd-kit consumes the touchmove that crosses the activation distance, leaving
 * the overlay at its origin until another touchmove arrives. Replaying that
 * activating event makes the strip follow the finger immediately. In a
 * scrollable bay, a short hold starts dragging while an immediate swipe keeps
 * the browser's native vertical scrolling available.
 */
class StripTouchSensor extends TouchSensorWithInternals {
  constructor(props: StripTouchSensorProps) {
    const activationConstraint = canScrollVertically(getScrollContainer(props.activeNode))
      ? { delay: TOUCH_DRAG_DELAY_MS, tolerance: TOUCH_DRAG_TOLERANCE_PX }
      : { distance: 0 };

    super({
      ...props,
      options: {
        ...props.options,
        activationConstraint,
      },
    });
  }

  handleMove(event: Event) {
    const wasActivated = this.activated;
    super.handleMove(event);

    if (!wasActivated && this.activated) {
      super.handleMove(event);
    }
  }
}

export function useStripSensors({ disabled = false }: { disabled?: boolean } = {}) {
  const mouseSensor = useSensor(MouseSensor, {
    activationConstraint: { distance: MOUSE_DRAG_DISTANCE },
  });
  const touchSensor = useSensor(StripTouchSensor);

  return useSensors(...(disabled ? [] : [mouseSensor, touchSensor]));
}
