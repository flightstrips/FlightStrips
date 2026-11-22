// Standard arrival taxi routing at EKCH, offered as the default clearance until the pilot types
// what ATC actually said. Same stand grouping as the default exit taxiway on arrival strips.
import { K3_EXIT_STANDS } from '@/components/strip/defaultExitTaxiway';

/** Default arrival clearance for a landing runway and stand, or null when there is none. */
export function defaultArrivalClearance(runway: string | null | undefined, stand: string | null | undefined): string | null {
  const rwy = (runway ?? '').trim().toUpperCase().replace(/^RWY/, '');
  const st = (stand ?? '').trim().toUpperCase();
  if (rwy === '22L') return K3_EXIT_STANDS.has(st) ? 'B.C' : 'B.30';
  if (rwy === '04L') return 'A.30';
  return null;
}
