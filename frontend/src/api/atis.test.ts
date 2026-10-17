import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchVatsimAtis, parseAtisWeather } from './atis';

const arrival = {
  callsign: 'EKCH_A_ATIS', atis_code: 'C', frequency: '122.755',
  text_atis: ['EKCH ARR ATIS C', 'RWY IN USE 22L'], last_updated: new Date().toISOString(),
};
const departure = {
  ...arrival, callsign: 'EKCH_D_ATIS', atis_code: 'D', frequency: '122.855',
  text_atis: ['EKCH DEP ATIS D', 'RWY IN USE 22R'],
};

afterEach(() => vi.unstubAllGlobals());

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

describe('VATSIM ATIS', () => {
  it('selects the exact arrival or departure callsign, not another station', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify([departure, arrival])));
    vi.stubGlobal('fetch', fetchMock);
    expect(await fetchVatsimAtis('ekch', false)).toMatchObject({ callsign: 'EKCH_A_ATIS', code: 'C', text: arrival.text_atis });
    fetchMock.mockResolvedValue(new Response(JSON.stringify([arrival, departure])));
    expect(await fetchVatsimAtis('EKCH', true)).toMatchObject({ callsign: 'EKCH_D_ATIS', code: 'D', text: departure.text_atis });
  });

  it('marks an old station update stale', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify([{ ...arrival, last_updated: '2020-01-01T00:00:00Z' }]))));
    expect((await fetchVatsimAtis('EKCH', false)).stale).toBe(true);
  });

  it.each([
    { body: [departure], error: 'EKCH_A_ATIS is offline', status: 200 },
    { body: {}, error: 'Invalid ATIS feed', status: 200 },
    { body: [{ ...arrival, text_atis: [12] }], error: 'Invalid ATIS data', status: 200 },
    { body: [{ ...arrival, last_updated: 'invalid' }], error: 'Invalid ATIS data', status: 200 },
    { body: [], error: 'ATIS feed unavailable (503)', status: 503 },
  ])('reports offline, malformed, or failed feeds explicitly: $error', async ({ body, error, status }) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status })));
    await expect(fetchVatsimAtis('EKCH', false)).rejects.toThrow(error);
  });
});
