import { MouseSensor, TouchSensor, useSensor, useSensors } from "@dnd-kit/core";

type StripTouchSensorProps = ConstructorParameters<typeof TouchSensor>[0];

const MOUSE_DRAG_DISTANCE = 5;
const TOUCH_DRAG_DELAY_MS = 150;
const TOUCH_DRAG_TOLERANCE_PX = 8;
const STRIP_SCROLL_CONTAINER_SELECTOR = '[data-strip-scroll-container="true"]';

function canScrollVertically(element: HTMLElement | null) {
  if (!element) {
    return false;
  }

  return element.scrollHeight > element.clientHeight + 1;
}

function getScrollContainer(activeNode: StripTouchSensorProps["activeNode"]) {
  const activatorNode = activeNode.activatorNode.current ?? activeNode.node.current;
  return activatorNode?.closest<HTMLElement>(STRIP_SCROLL_CONTAINER_SELECTOR) ?? null;
}

class StripTouchSensor extends TouchSensor {
  constructor(props: StripTouchSensorProps) {
    const scrollContainer = getScrollContainer(props.activeNode);
    const activationConstraint = canScrollVertically(scrollContainer)
      ? {
          delay: TOUCH_DRAG_DELAY_MS,
          tolerance: TOUCH_DRAG_TOLERANCE_PX,
        }
      : {
          distance: 0,
        };

    super({
      ...props,
      options: {
        ...props.options,
        activationConstraint,
      },
    });
  }
}

export function useStripSensors({ disabled = false }: { disabled?: boolean } = {}) {
  const mouseSensor = useSensor(MouseSensor, {
    activationConstraint: { distance: MOUSE_DRAG_DISTANCE },
  });
  const touchSensor = useSensor(StripTouchSensor);

  return useSensors(...(disabled ? [] : [mouseSensor, touchSensor]));
}
