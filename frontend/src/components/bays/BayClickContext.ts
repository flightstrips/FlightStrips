import { createContext, useContext } from 'react';

interface BayClickContextValue {
  onBayClick: (bayId: string) => void;
}

const noop = () => {};

export const BayClickContext = createContext<BayClickContextValue>({
  onBayClick: noop,
});

export function useBayClick(): BayClickContextValue {
  return useContext(BayClickContext);
}
