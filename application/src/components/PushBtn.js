// MyButton.tsx
import {extendVariants, Button} from "@nextui-org/react";

export const PushBtn = extendVariants(Button, {
  variants: {
    // <- modify/add variants
    color: {
      gray: "text-[#000] bg-[#d6d6d6]",
    },
    size: {
      xs: "px-unit-2 min-w-unit-12 h-unit-6 text-tiny gap-unit-1 rounded-none font-bold",
    },
  },
  defaultVariants: { // <- modify/add default variants
    color: "gray",
    size: "xs",
  },
  compoundVariants: [ // <- modify/add compound variants
    {
      color: "xs",
      class: "bg-[#84cc16]/80 opacity-100",
    },
  ],
});