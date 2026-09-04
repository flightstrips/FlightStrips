import { describe, expect, it } from 'vitest';
import type { FrontendController } from '@/api/models.ts';
import { hasCtwrOnline, hasLowerPositionOnline } from './store-hooks.ts';

const controller = (
  callsign: string,
  position: string,
  ownedSectors: string[],
  observer = false,
): FrontendController => ({
  callsign,
  position,
  identifier: '',
  section: '',
  owned_sectors: ownedSectors,
  observer,
});

describe('hasLowerPositionOnline', () => {
  const ownCallsign = 'EKCH_A_TWR';
  const ownPosition = '118.100';

  it('ignores cross-coupled lower-sector coverage from the same controller', () => {
    expect(hasLowerPositionOnline([
      controller(ownCallsign, '121.900', ['DEL']),
    ], ownPosition, ownCallsign)).toBe(false);
  });

  it('ignores observer positions with lower-sector coverage', () => {
    expect(hasLowerPositionOnline([
      controller('EKCH_OBS', '121.900', ['DEL']),
      controller('FR_OBS', '121.600', ['AA']),
      controller('EKCH_OBS_APP', '121.700', ['AD'], true),
    ], ownPosition, ownCallsign)).toBe(false);
  });

  it('ignores an observer using the CTWR frequency', () => {
    expect(hasCtwrOnline([
      controller('EKCH_OBS_APP', '118.580', [], true),
    ], ownPosition, ownCallsign)).toBe(false);
  });

  it('detects a distinct operational controller on the CTWR frequency', () => {
    expect(hasCtwrOnline([
      controller('EKCH_C_TWR', '118.580', []),
    ], ownPosition, ownCallsign)).toBe(true);
  });

  it('detects a distinct operational controller on a lower position', () => {
    expect(hasLowerPositionOnline([
      controller('EKCH_DEL', '121.900', ['DEL']),
    ], ownPosition, ownCallsign)).toBe(true);
  });
});
