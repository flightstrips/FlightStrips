import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import EFBPage from './efb';

vi.mock('@auth0/auth0-react', () => ({
  useAuth0: () => ({ getAccessTokenSilently: vi.fn().mockResolvedValue('token') }),
}));

function jsonResponse(body: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  }));
}

describe('EFB page interactions', () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('keeps the local callsign control and gives the no-flight artwork real destinations', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => jsonResponse({ live_mode: false, online_callsign: null })));
    render(<EFBPage />);

    expect(await screen.findByLabelText('Development callsign')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'L1 Picture' })).toHaveAttribute('href', 'https://www.simbrief.com/home/');
    expect(screen.getByRole('link', { name: 'R1 Picture' })).toHaveAttribute('href', 'https://my.vatsim.net/pilots/flightplan');
    expect(await screen.findByRole('button', { name: 'CLICK TO REFRESH' })).toBeInTheDocument();

    const nilTiles = screen.getAllByAltText('Loading');
    for (const tile of nilTiles) fireEvent.click(tile);
    expect(screen.queryByText(/unavailable/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Details for/)).not.toBeInTheDocument();
  });

  it('uses NIL for unavailable operational fields and does not open an info placeholder', async () => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/efb/me')) return jsonResponse({ live_mode: false, online_callsign: null });
      return jsonResponse({
        callsign: 'SAS456', aircraft_type: 'A320', origin: 'EKCH', destination: 'ESSA', phase: 'DEPARTURE',
        stand: null, stand_version: null, cleared_altitude: null, ctot: null, runway: null, sid: null,
        pdc_state: '', pdc_requires_pilot_action: false, pdc_available: false, pdc_can_submit: false,
        pdc_clearance_text: null, tobt: null, eobt: null, tsat: null, atis: null,
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);

    const callsign = await screen.findByLabelText('Development callsign');
    fireEvent.change(callsign, { target: { value: 'sas456' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));

    expect(await screen.findByText('SAS456')).toBeInTheDocument();
    expect(screen.queryByText('124.980')).not.toBeInTheDocument();
    expect(screen.queryByText('NEXEN2A')).not.toBeInTheDocument();
    expect(screen.queryByText('Details for M2 box')).not.toBeInTheDocument();

    fireEvent.click(screen.getByAltText('Departure Info'));
    await waitFor(() => expect(screen.queryByText(/Details for/)).not.toBeInTheDocument());
  });

  it('shows the observed stand, formats TSAT, uses the base PDC aircraft type, and hides unavailable contact passing', async () => {
    const flight = {
      callsign: 'SAS789', aircraft_type: 'A320/H-SDE2E3FGHIRWXY/LB1', origin: 'EKCH', destination: 'ESSA', phase: 'DEPARTURE',
      stand: 'A12', stand_version: null, cleared_altitude: 7000, ctot: null, runway: '22R', sid: 'NEXEN2A',
      departure_frequency: null, pdc_state: '', pdc_requires_pilot_action: false, pdc_available: true, pdc_can_submit: true,
      pdc_clearance_text: null, tobt: '1200', eobt: '1200', tsat: '123000', atis: { code: 'A', text: ['ATIS A'] },
      capabilities: { pdc: true, tobt_update: true, stand_reassignment: false },
    };
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/efb/me')) return jsonResponse({ live_mode: false, online_callsign: null });
      if (url.includes('/api/pdc/request')) return jsonResponse({});
      return jsonResponse(flight);
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);

    fireEvent.change(await screen.findByLabelText('Development callsign'), { target: { value: 'sas789' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));

    expect(await screen.findByText('SAS789')).toBeInTheDocument();
    expect(screen.getByText('A12')).toBeInTheDocument();
    expect(screen.getByText('1230')).toBeInTheDocument();
    expect(screen.queryByText('123000')).not.toBeInTheDocument();
    expect(screen.queryByText('CONTACT PASSING')).not.toBeInTheDocument();

    fireEvent.click(screen.getByAltText('Charts'));
    expect(screen.getByRole('dialog', { name: 'CHARTS' })).toBeInTheDocument();
    expect(screen.getByText(/EKCH, runway 22R, SID NEXEN2A/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'CLICK TO CLOSE' }));

    fireEvent.click(screen.getByText('REQ PDC'));
    await waitFor(() => {
      const request = fetchMock.mock.calls.find(([input]) => String(input).includes('/api/pdc/request'));
      expect(request).toBeDefined();
      expect(JSON.parse(request![1].body)).toMatchObject({ aircraft_type: 'A320' });
    });
  });

  it('marks the arrival briefing as coming soon without opening a briefing dialog', async () => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: false, online_callsign: null });
      return jsonResponse({
        callsign: 'SAS790', aircraft_type: 'A320', origin: 'ESSA', destination: 'EKCH', phase: 'ARRIVAL',
        stand: 'A12', runway: '22L', sid: null, pdc_state: '', pdc_requires_pilot_action: false,
        pdc_available: false, pdc_can_submit: false, capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);

    fireEvent.change(await screen.findByLabelText('Development callsign'), { target: { value: 'sas790' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));

    expect(await screen.findByText('COMING SOON')).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText('Arrival briefing coming soon'));
    expect(screen.queryByRole('dialog', { name: /brief/i })).not.toBeInTheDocument();
  });

  it('shows API failures as retryable errors instead of pretending there is no flight', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => jsonResponse({ error: 'pilot lookup unavailable' }, 503)));
    render(<EFBPage />);

    expect(await screen.findByRole('button', { name: 'CLICK TO RETRY' })).toBeInTheDocument();
    expect(screen.getByText('pilot lookup unavailable')).toBeInTheDocument();
    expect(screen.queryByText('NO FLIGHTPLAN')).not.toBeInTheDocument();
  });

  it('marks stale ATIS and prevents a PDC request', async () => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: false, online_callsign: null });
      return jsonResponse({
        callsign: 'SAS791', aircraft_type: 'A320', origin: 'EKCH', destination: 'ESSA', phase: 'DEPARTURE',
        stand: 'A12', runway: '22R', pdc_state: '', pdc_requires_pilot_action: false,
        pdc_available: true, pdc_can_submit: true, atis: { code: 'A', text: ['ATIS A'], stale: true },
        capabilities: { pdc: true, tobt_update: true, stand_reassignment: false },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);

    fireEvent.change(await screen.findByLabelText('Development callsign'), { target: { value: 'sas791' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));

    expect(await screen.findByText('ATIS A STALE')).toBeInTheDocument();
    expect(screen.getByText('REQ ATIS')).toBeInTheDocument();
    fireEvent.click(screen.getByText('REQ ATIS'));
    expect(await screen.findByText('Current ATIS is required before requesting PDC')).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/api/pdc/request'))).toBe(false);
  });
});
