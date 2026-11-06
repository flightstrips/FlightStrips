import { describe, expect, it, vi } from 'vitest';
import { fetchCachedAtis, parseAtisWeather } from './atis';

describe('ATIS weather widgets', () => {
  it.each([
    ['WIND 360 16 KT VIS 10KM T 11 DP 7 QNH 1021', { windDirection: '360', temperature: 11, qnh: 1021 }],
    ['wind 240deg 12kt temperature minus 3 qnh1013', { windDirection: '240', temperature: -3, qnh: 1013 }],
    ['27015G25KT 08/04 Q1009', { windDirection: '270', temperature: 8, qnh: 1009 }],
    ['VRB03KT M02/M05 Q0998', { windDirection: 'VRB', temperature: -2, qnh: 998 }],
    ['WIND VARIABLE 3 KT TEMP -4 QNH 1002', { windDirection: 'VRB', temperature: -4, qnh: 1002 }],
    ['WIND CALM T 00 QNH 1020', { windDirection: 'CALM', temperature: 0, qnh: 1020 }],
    ['ATIS A RWY 22R CONDITION REPORT AT 1750Z', { windDirection: null, temperature: null, qnh: null }],
    ['WIND 390 16 KT', { windDirection: null, temperature: null, qnh: null }],
    ['', { windDirection: null, temperature: null, qnh: null }],
  ])('extracts reported weather without inventing missing values: %s', (text, expected) => {
    expect(parseAtisWeather(text)).toMatchObject(expected);
  });

  it.each([
    ['WIND 360/16 KT', '360', '16 KT'],
    ['WIND 180/08KT', '180', '8 KT'],
    ['WIND 090 12 GUSTING 22 KT', '090', '12G22 KT'],
    ['27015G25KT', '270', '15G25 KT'],
    ['VRB03KT', 'VRB', '3 KT'],
    ['WIND CALM', 'CALM', 'CALM'],
    ['', null, null],
  ])('extracts wind strength and gusts: %s', (text, windDirection, windStrength) => {
    expect(parseAtisWeather(text)).toMatchObject({ windDirection, windStrength });
  });
});

describe('backend ATIS cache', () => {
  it('reads the selected airport and direction through authenticated backend consumption', async () => {
    const observation = { callsign: 'AAAA_A_ATIS', code: 'C', text: ['Synthetic ATIS'], frequency: '118.000', last_updated: new Date().toISOString(), stale: false };
    const request = vi.fn().mockResolvedValue(observation);
    expect(await fetchCachedAtis('aaaa', false, request)).toEqual(observation);
    expect(request).toHaveBeenCalledWith('/api/weather/atis?airport=AAAA&departure=false', { signal: undefined });
    request.mockResolvedValue({ ...observation, stale: true });
    expect((await fetchCachedAtis('AAAA', true, request)).stale).toBe(true);
    expect(request).toHaveBeenLastCalledWith('/api/weather/atis?airport=AAAA&departure=true', { signal: undefined });
  });
  it('reports unavailable and malformed cached observations', async () => {
    await expect(fetchCachedAtis('AAAA', false, vi.fn().mockRejectedValue(new Error('ATIS offline')))).rejects.toThrow('ATIS offline');
    await expect(fetchCachedAtis('AAAA', false, vi.fn().mockResolvedValue({}))).rejects.toThrow('Invalid cached ATIS');
  });
});
