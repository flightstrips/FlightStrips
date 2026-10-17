import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import EFBPage from './efb';

vi.mock('@auth0/auth0-react', () => {
  const auth = { getAccessTokenSilently: vi.fn().mockResolvedValue('token') };
  return { useAuth0: () => auth };
});

function jsonResponse(body: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  }));
}

function previewResponse(input: RequestInfo | URL) {
  if (String(input).includes('afv-atis-data.json')) {
    return jsonResponse([{
      callsign: 'EKCH_A_ATIS', atis_code: 'C', frequency: '122.755', last_updated: new Date().toISOString(),
      text_atis: ['EKCH ARR ATIS C', 'RUNWAY 22L CONDITION REPORT AT 1750Z', 'END OF ATIS C'],
    }]);
  }
  return jsonResponse({ error: 'EFB API unavailable' }, 503);
}

describe('EFB page interactions', () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it.each([
    ['ASSIGNED', 'bg-[#1A475F]'],
    ['SELECTED', 'bg-[#1A475F]'],
    ['CONFIRMED', 'bg-[#41826E]'],
    ['OVERWRITTEN', 'bg-[#41826E]'],
    ['MISMATCH', 'bg-[#B63F3F]'],
    ['EXPIRED', 'bg-[#2E343D]'],
    ['NO STAND DETECTED', 'bg-[#2E343D]'],
    ['STAND ASSIGNMENT UNAVAILABLE', 'bg-[#2E343D]'],
  ])('renders departure stand state %s with matching fills', async (state, color) => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase: 'DEPARTURE',
        stand: state === 'NO STAND DETECTED' ? null : 'A12', stand_state: state,
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    await screen.findByText('SAS123');
    expect(screen.getByRole('group', { name: 'Stand' })).toHaveClass(color);
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveClass(color);
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveClass('efb-action-bar');
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveTextContent(state);
  });

  it('initializes an owned prefile on first opening and does not reopen its hold', async () => {
    let opened = false;
    const requests: string[] = [];
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: null });
      if (url.includes('/api/efb/stand/open')) {
        requests.push('open');
        expect(init?.method).toBe('POST');
        opened = true;
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      requests.push('flight');
      return jsonResponse({
        callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase: 'DEPARTURE', flight_state: 'prefile',
        stand: opened ? 'A12' : null, stand_state: opened ? 'ASSIGNED' : 'STAND ASSIGNMENT UNAVAILABLE',
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: opened, stand_initialization: !opened },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    const view = render(<EFBPage />);
    expect(await screen.findByText('A12')).toBeInTheDocument();
    expect(requests).toEqual(['flight', 'open', 'flight']);
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveTextContent('ASSIGNED');
    view.unmount();
    render(<EFBPage />);
    expect(await screen.findByText('A12')).toBeInTheDocument();
    expect(requests.filter((request) => request === 'open')).toHaveLength(1);
  });

  it('keeps the flight visible and reports stand initialization failures', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      if (url.includes('/api/efb/stand/open')) return jsonResponse({ error: 'No compatible stand is available' }, 409);
      return jsonResponse({
        callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase: 'DEPARTURE', flight_state: 'prefile',
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false, stand_initialization: true },
      });
    }));
    render(<EFBPage />);
    expect(await screen.findByText('SAS123')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('No compatible stand is available');
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveTextContent('STAND ASSIGNMENT UNAVAILABLE');
  });

  it.each([
    ['ARRIVAL', false],
    ['DEPARTURE', true],
  ])('does not initialize %s stands with read-only=%s', async (phase, readOnly) => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase, read_only: readOnly,
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false, stand_initialization: true },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    await screen.findByText('SAS123');
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/api/efb/stand/open'))).toBe(false);
  });

  it.each([
    ['DEPARTURE', ['Stand', 'Departure information', 'Charts', 'ATIS / PDC', 'TOBT / TSAT / CTOT', 'Departure briefing']],
    ['ARRIVAL', ['Charts', 'Arrival information', 'Arrival briefing', 'TOBT / TSAT / CTOT', 'ATIS', 'Stand']],
  ] as const)('opens tile-local information without triggering %s tile actions', async (phase, titles) => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign: 'SAS123', origin: phase === 'ARRIVAL' ? 'EDDF' : 'EKCH',
        destination: phase === 'ARRIVAL' ? 'EKCH' : 'EDDF', phase,
        capabilities: { pdc: true, tobt_update: true, stand_reassignment: true },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    await screen.findByText('SAS123');
    expect(screen.getAllByRole('button', { name: / information$/ })).toHaveLength(6);
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveClass('efb-action-bar', 'efb-clickspot');

    for (const title of titles) {
      const label = title.endsWith('information') ? title : `${title} information`;
      const button = screen.getByRole('button', { name: label });
      const tile = button.closest('[data-efb-tile]')!;
      const infoMargin = button.closest('[data-efb-tile-info]')!;
      expect(infoMargin).toHaveTextContent(/^i$/);
      expect(infoMargin).toHaveClass('bottom-full');
      const content = tile.querySelector('[data-efb-tile-content]');
      expect(content).toHaveClass('inset-0');
      expect(content).not.toContainElement(button);
      fireEvent.click(infoMargin);
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      fireEvent.mouseEnter(tile);
      const fullTileEffect = (phase === 'ARRIVAL' ? ['L2', 'R2'] : ['L3', 'R3']).includes(tile.getAttribute('data-efb-tile') ?? '');
      expect(tile.classList.contains('efb-tile-hover')).toBe(fullTileEffect);
      expect(tile).not.toHaveClass('scale-[1.015]');
      fireEvent.mouseEnter(infoMargin);
      expect(tile).not.toHaveClass('efb-tile-hover');
      fireEvent.mouseLeave(infoMargin, { relatedTarget: tile });
      expect(tile.classList.contains('efb-tile-hover')).toBe(fullTileEffect);
      fireEvent.mouseEnter(button);
      expect(tile).not.toHaveClass('efb-tile-hover');
      fireEvent.mouseLeave(button, { relatedTarget: tile });
      expect(tile.classList.contains('efb-tile-hover')).toBe(fullTileEffect);
      fireEvent.click(button);
      const dialog = screen.getByRole('dialog', { name: label });
      expect(screen.getAllByRole('dialog')).toHaveLength(1);
      expect(dialog.closest('[data-efb-tile]')).toBe(button.closest('[data-efb-tile]'));
      if (phase === 'ARRIVAL' && title === 'Stand') {
        expect(dialog).toHaveTextContent('This shows your arrival stand assigned by ATC.');
        expect(dialog).toHaveTextContent('your fellow pilots adhere to their stand assignments');
        expect(dialog).toHaveTextContent('click to request a stand');
        expect(dialog).toHaveTextContent("That's everything in green.");
        expect(dialog.querySelectorAll('p')).toHaveLength(3);
        expect(within(dialog).getAllByRole('separator')).toHaveLength(2);
      } else if (phase === 'ARRIVAL') {
        const expected = {
          Charts: ['Navigraph', 'ChartFox', 'NO excuse', 2],
          'Arrival information': ['CAREFULLY', 'correct version in my FMC', 'DO NOT turn inbound', 4],
          'Arrival briefing': ['properly prepared', 'specific arrival', 'abbreviated form', 2],
          'TOBT / TSAT / CTOT': ['VITAL', 'UNCONTROLLED', 'YOUR job', 3],
          ATIS: ['latest ATIS', 'crosswind', 'first contact', 2],
        } as const;
        const content = expected[title as keyof typeof expected];
        for (const text of content.slice(0, 3)) expect(dialog).toHaveTextContent(String(text));
        expect(dialog.querySelectorAll('p')).toHaveLength(content[3]);
        expect(within(dialog).getAllByRole('separator')).toHaveLength(content[3] - 1);
      } else {
        const expected = {
          Stand: ['ALL conditions', 'PLEASE USE THIS STAND IF POSSIBLE', 'available and suitable', 3],
          'Departure information': ['SID, initial climb and CTOTs', 'TWR will NOT give you the frequency', 'Once you pass 1000 ft', 5],
          Charts: ['ChartFox', 'charts for your departure', 'NO excuse', 2],
          'ATIS / PDC': ['latest ATIS', 'request your PDC', 'must view the ATIS first', 2],
          'TOBT / TSAT / CTOT': ['UPDATE TOBT', '5-minute window', 'must call us for your startup', 3],
          'Departure briefing': ['properly prepared', 'specific departure', 'abbreviated form', 2],
        } as const;
        const content = expected[title as keyof typeof expected];
        for (const text of content.slice(0, 3)) expect(dialog).toHaveTextContent(String(text));
        expect(dialog.querySelectorAll('p')).toHaveLength(content[3]);
        expect(within(dialog).getAllByRole('separator')).toHaveLength(content[3] - 1);
      }
      expect(dialog).toHaveClass('-inset-[2px]', 'rounded-none', 'bg-[#000109]', 'animate-[efb-info-expand_50ms_ease-out]');
      expect(button).toHaveAttribute('aria-expanded', 'true');
      expect(tile).not.toHaveClass('efb-tile-hover');
      const close = within(dialog).getByRole('button', { name: `Close ${label.toLowerCase()}` });
      expect(button).toHaveClass('border', 'border-[#FF9800]');
      expect(button).not.toHaveClass('border-2');
      for (const element of [dialog, close]) {
        expect(element).toHaveClass('border-2', 'border-[#FF9800]');
      }
      expect(close).toHaveFocus();
      fireEvent.click(dialog);
      expect(dialog).toBeInTheDocument();
      fireEvent.keyDown(close, { key: 'Escape' });
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      expect(button).toHaveFocus();
      expect(button).toHaveAttribute('aria-expanded', 'false');
      fireEvent.click(button);
      fireEvent.click(screen.getByRole('button', { name: `Close ${label.toLowerCase()}` }));
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    }
    expect(fetchMock.mock.calls.some(([url]) => /\/(?:stands|tobt|stand|request)(?:\?|$)/.test(String(url)))).toBe(false);
  });

  it('loads a manually selected prefile as a read-only VATSIM flight', async () => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/efb/me')) return jsonResponse({ live_mode: false, callsign_lookup: true, online_callsign: null });
      return jsonResponse({
        callsign: 'SAS99X', origin: 'EKCH', destination: 'EKYT', phase: 'DEPARTURE',
        read_only: true, flight_state: 'prefile', tobt: '1935', tsat: '1940', ctot: '1950',
        stand: null, pdc_available: false, pdc_can_submit: false, pdc_state: '',
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    const input = await screen.findByRole('textbox', { name: 'VATSIM callsign' });
    fireEvent.change(input, { target: { value: 'sas99x' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));
    expect(await screen.findByText('SAS99X')).toBeInTheDocument();
    expect(screen.getByRole('status', { name: 'VATSIM callsign lookup' })).toHaveTextContent('SAS99X - PREFILED - EKCH TO EKYT');
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith('/api/efb/flight?callsign=SAS99X'))).toBe(true);
    expect(screen.getByText('1935')).toBeInTheDocument();
    expect(screen.queryByText('CLICK TO UPDATE')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'UPDATE TOBT' })).toBeDisabled();
    expect(screen.getByRole('group', { name: 'Stand' })).toHaveTextContent('TBA');
  });

  it('selects Sweatbox and scopes TOBT writes before switching back to read-only VATSIM', async () => {
    let updated = false;
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://localhost');
      if (url.pathname.endsWith('/me')) return jsonResponse({ live_mode: false, callsign_lookup: true, training_available: true });
      if (url.pathname.endsWith('/training-sessions')) return jsonResponse({ sessions: [{ id: 8, name: 'Sweatbox', airport: 'EKCH' }] });
      const training = url.searchParams.get('training_session') === '8';
      if (url.pathname.endsWith('/tobt')) {
        expect(training).toBe(true);
        expect(init?.method).toBe('PUT');
        expect(JSON.parse(String(init?.body))).toEqual({ callsign: 'SAS123', tobt: '2130' });
        updated = true;
        return jsonResponse({});
      }
      return jsonResponse({
        callsign: 'SAS123', origin: 'EKCH', destination: 'EKYT', phase: 'DEPARTURE',
        tobt: updated ? '2130' : '2100', read_only: !training, flight_state: 'prefile',
        capabilities: { pdc: false, tobt_update: training, stand_reassignment: training },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    await screen.findByRole('option', { name: 'Sweatbox - EKCH (#8)' });
    fireEvent.change(screen.getByLabelText('EFB data source'), { target: { value: '8' } });
    fireEvent.change(screen.getByLabelText('Sweatbox callsign'), { target: { value: 'sas123' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));
    expect(await screen.findByText('SAS123')).toBeInTheDocument();
    expect(screen.getByRole('status', { name: 'Sweatbox training' })).toHaveTextContent('SESSION 8');
    expect(screen.queryByRole('status', { name: 'VATSIM callsign lookup' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'UPDATE TOBT' })).toBeEnabled();
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    const menu = screen.getByRole('dialog', { name: 'TOBT' });
    fireEvent.change(within(menu).getByLabelText('New TOBT (UTC)'), { target: { value: '2130' } });
    fireEvent.click(within(menu).getByRole('button', { name: 'UPDATE TOBT' }));
    expect(await screen.findByText('2130')).toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'TOBT' })).toHaveClass('bg-[#41826E]');
    expect(screen.getByRole('button', { name: 'TOBT CONFIRMED' })).toHaveClass('bg-[#41826E]');
    expect(window.sessionStorage.getItem('efb-training-session')).toBe('8');
    fireEvent.change(screen.getByLabelText('EFB data source'), { target: { value: '' } });
    await waitFor(() => expect(screen.getByRole('button', { name: 'UPDATE TOBT' })).toBeDisabled());
    expect(screen.getByRole('status', { name: 'VATSIM callsign lookup' })).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith('/api/efb/flight?callsign=SAS123'))).toBe(true);
  });

  it.each([
    { phase: 'ARRIVAL', unsuitable: false }, { phase: 'DEPARTURE', unsuitable: false },
    { phase: 'ARRIVAL', unsuitable: true }, { phase: 'DEPARTURE', unsuitable: true },
  ])('routes Sweatbox $phase stand assignments with unsuitable=$unsuitable to the selected session', async ({ phase, unsuitable }) => {
    window.sessionStorage.setItem('efb-training-session', '8');
    window.sessionStorage.setItem('efb-test-callsign', 'SAS123');
    let assigned = false;
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://localhost');
      if (url.pathname.endsWith('/me')) return jsonResponse({ live_mode: false, callsign_lookup: true, training_available: true });
      if (url.pathname.endsWith('/training-sessions')) return jsonResponse({ sessions: [{ id: 8, name: 'Sweatbox', airport: 'EKCH' }] });
      expect(url.searchParams.get('training_session')).toBe('8');
      if (url.pathname.endsWith('/stands')) {
        expect(url.searchParams.get('callsign')).toBe('SAS123');
        return jsonResponse({ stands: [{ stand: 'B4', available: true }, { stand: 'C27', available: !unsuitable, unsuitable, selectable: true }] });
      }
      if (url.pathname.endsWith('/stand')) {
        expect(init?.method).toBe('PUT');
        expect(JSON.parse(String(init?.body))).toEqual({ callsign: 'SAS123', stand: 'C27', version: phase === 'DEPARTURE' ? 0 : 3, ...(unsuitable ? { allow_unsuitable: true } : {}) });
        assigned = true;
        return jsonResponse({ stand: 'C27', stand_state: 'ASSIGNED', version: 4 });
      }
      return jsonResponse({
        callsign: 'SAS123', origin: phase === 'DEPARTURE' ? 'EKCH' : 'EDDF', destination: phase === 'DEPARTURE' ? 'EDDF' : 'EKCH', phase,
        stand: assigned ? 'C27' : phase === 'DEPARTURE' ? null : 'B4', stand_state: assigned ? 'ASSIGNED' : 'ESTIMATED', stand_version: assigned ? 4 : phase === 'DEPARTURE' ? null : 3,
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: true },
      });
    }));
    render(<EFBPage />);
    fireEvent.change(await screen.findByLabelText('Sweatbox callsign'), { target: { value: 'SAS123' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));
    const standHeader = await screen.findByRole('group', { name: 'Stand' });
    expect(standHeader).toHaveClass('efb-action-bar', 'efb-clickspot');
    fireEvent.click(screen.getByRole('status', { name: 'Stand state' }));
    await screen.findByRole('button', { name: 'Close stand selection' });
    fireEvent.click(screen.getByRole('button', { name: 'Close stand selection' }));
    fireEvent.click(within(standHeader).getByRole('button', { name: 'Open stand dialog' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Stand C27' })).toBeEnabled());
    fireEvent.click(screen.getByRole('button', { name: 'Stand C27' }));
    fireEvent.click(screen.getByRole('button', { name: 'REQUEST NEW STAND' }));
    if (unsuitable) {
      expect(assigned).toBe(false);
      fireEvent.click(screen.getByRole('button', { name: 'SELECT ANYWAY' }));
    }
    await waitFor(() => expect(assigned).toBe(true));
    expect(screen.getByRole('group', { name: 'Stand' })).toHaveTextContent('C27');
  });

  it('keeps a stopped Sweatbox session selected without falling back to VATSIM', async () => {
    window.sessionStorage.setItem('efb-training-session', '8');
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/me')) return jsonResponse({ live_mode: false, callsign_lookup: true, training_available: true });
      if (url.includes('/training-sessions')) return jsonResponse({ sessions: [] });
      return jsonResponse({ error: 'Sweatbox session unavailable' }, 403);
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    fireEvent.change(await screen.findByLabelText('Sweatbox callsign'), { target: { value: 'SAS123' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));
    expect(await screen.findByText('Sweatbox session unavailable', { selector: 'option' })).toBeInTheDocument();
    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/flight?training_session=8'))).toBe(true));
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/flight?callsign='))).toBe(false);
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
    expect(screen.getByRole('group', { name: 'Stand' })).toHaveTextContent('TBA');
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveTextContent('TBA');
    expect(screen.queryByText('124.980')).not.toBeInTheDocument();
    expect(screen.queryByText('NEXEN2A')).not.toBeInTheDocument();
    expect(screen.queryByText('Details for M2 box')).not.toBeInTheDocument();

    fireEvent.click(screen.getByAltText('Departure Info'));
    await waitFor(() => expect(screen.queryByText(/Details for/)).not.toBeInTheDocument());
  });

  it('loads the VATSIM account flight without a strip and shows optional integration failures', async () => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS99X', callsign_locked: true });
      return jsonResponse({
        callsign: 'SAS99X', origin: 'EKCH', destination: 'EKYT', phase: 'DEPARTURE',
        stand: null, tobt: null, tsat: null, ctot: null,
        data_errors: ['vIFF timing is unavailable; TOBT updates are disabled.'],
        pdc_available: false, pdc_can_submit: false, pdc_state: '',
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    expect(await screen.findByText('SAS99X')).toBeInTheDocument();
    expect(screen.queryByLabelText('Development callsign')).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Stand' })).toHaveTextContent('TBA');
    expect(screen.getByRole('alert')).toHaveTextContent('vIFF timing is unavailable');
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/api/efb/flight?'))).toBe(false);
    expect(screen.queryByText('CLICK TO UPDATE')).not.toBeInTheDocument();
    expect(screen.queryByRole('dialog', { name: 'TOBT' })).not.toBeInTheDocument();
  });

  it.each([
    ['WIND 360/16 KT T 11 DP 7 QNH 1021', ['16 KT', '11 °C', '1021 hPa'], '04L', 320],
    ['WIND 360/16 KT T 11 DP 7 QNH 1021', ['16 KT', '11 °C', '1021 hPa'], '04R', 320],
    ['WIND 180/08 KT T 11 DP 7 QNH 1021', ['8 KT', '11 °C', '1021 hPa'], '22L', 320],
    ['WIND 180/08 KT T 11 DP 7 QNH 1021', ['8 KT', '11 °C', '1021 hPa'], '22R', 320],
    ['WIND 360/16 KT T 11 DP 7 QNH 1021', ['16 KT', '11 °C', '1021 hPa'], '22R', 140],
    ['WIND 360/16 KT T 11 DP 7 QNH 1021', ['16 KT', '11 °C', '1021 hPa'], null, 0],
    ['ATIS A RWY 22R', ['N/A', 'N/A', 'N/A'], '22R', null],
  ] as const)('shows transparent runway-relative weather widgets: %s / %s / %s', async (atis, values, runway, rotation) => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase: 'DEPARTURE',
        runway, atis: { code: 'A', text: [atis] },
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    const weather = await screen.findByRole('group', { name: 'ATIS weather' });
    expect(weather).toHaveClass('top-1/2', 'h-[30%]', 'grid-cols-3');
    expect(screen.getByAltText('ATIS').closest('.efb-clickspot')).toHaveClass('efb-action-bar');
    for (const [index, label] of ['WIND', 'TEMPERATURE', 'QNH'].entries()) {
      const widget = within(weather).getByRole('group', { name: label });
      expect(widget).toHaveClass('bg-transparent');
      expect(widget).toHaveTextContent(values[index]);
    }
    fireEvent.click(weather);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    const direction = atis.includes('360') ? 360 : atis.includes('180') ? 180 : null;
    if (direction !== null) {
      expect(within(weather).getByRole('img', { name: `Wind from ${direction} degrees` })).toHaveStyle({ transform: `rotate(${rotation}deg)` });
    } else {
      expect(within(weather).queryByRole('img')).not.toBeInTheDocument();
    }
  });

  it.each(['DEPARTURE', 'ARRIVAL'])('acknowledges only viewed current ATIS for %s and resets on new information', async (phase) => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    let code = 'A';
    let message = 'WIND 360/16 KT';
    let lastUpdated = '2026-10-17T10:00:00Z';
    let callsign = 'SAS123';
    let stale = false;
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign, origin: phase === 'ARRIVAL' ? 'EDDF' : 'EKCH',
        destination: phase === 'ARRIVAL' ? 'EKCH' : 'EDDF', phase,
        atis: { code, text: [`ATIS ${code}`, message], last_updated: lastUpdated, stale },
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    await screen.findByText('SAS123');
    const identifier = () => phase === 'ARRIVAL'
      ? screen.getByRole('group', { name: 'ATIS identifier' })
      : screen.getByAltText('ATIS').closest('.efb-clickspot')!;
    const viewAtis = async () => {
      fireEvent.click(identifier());
      await screen.findByText(`${stale ? 'STALE ' : ''}ATIS for ${callsign}`);
      fireEvent.click(screen.getByRole('button', { name: 'CLOSE' }));
    };
    const refresh = async () => { await act(async () => { await vi.advanceTimersByTimeAsync(15_000); }); };
    expect(identifier()).toHaveClass('bg-[#1A475F]');
    await refresh();
    expect(identifier()).toHaveClass('bg-[#1A475F]');
    await viewAtis();
    expect(identifier()).toHaveClass('bg-[#41826E]');
    lastUpdated = '2026-10-17T10:01:00Z';
    await refresh();
    expect(identifier()).toHaveClass('bg-[#41826E]');
    code = 'B';
    await refresh();
    expect(identifier()).toHaveTextContent('ATIS B');
    expect(identifier()).toHaveClass('bg-[#1A475F]');
    await viewAtis();
    expect(identifier()).toHaveClass('bg-[#41826E]');
    message = 'WIND 180/20 KT';
    await refresh();
    expect(identifier()).toHaveClass('bg-[#1A475F]');
    await viewAtis();
    expect(identifier()).toHaveClass('bg-[#41826E]');
    callsign = 'SAS456';
    await refresh();
    expect(identifier()).toHaveClass('bg-[#1A475F]');
    await viewAtis();
    expect(identifier()).toHaveClass('bg-[#41826E]');
    stale = true;
    await refresh();
    await viewAtis();
    expect(identifier()).toHaveClass('bg-[#1A475F]');
  });

  it.each(['ATIS identifier', 'ATIS weather'])('opens the shared dialog from arrival %s with arrival weather and text', async (clickTarget) => {
    const arrivalAtis = 'EKCH ARR ATIS B WIND 360/16 KT T 11 DP 7 QNH 1021';
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS676' });
      return jsonResponse({
        callsign: 'SAS676', origin: 'EDDF', destination: 'EKCH', phase: 'ARRIVAL', on_ground: false,
        runway: '22L', atis: { code: 'B', text: [arrivalAtis] },
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    const atisButton = await screen.findByRole('button', { name: 'Open ATIS' });
    expect(atisButton.closest('[data-efb-tile]')).toHaveAttribute('data-efb-tile', 'M3');
    expect(atisButton).toHaveClass('efb-atis-control');
    expect(atisButton).not.toHaveClass('efb-clickspot');
    expect(within(atisButton).getByRole('group', { name: 'ATIS identifier' })).toHaveClass('efb-action-bar', 'efb-clickspot');
    expect(within(atisButton).getByText('ATIS B')).toHaveClass('text-[clamp(14px,5.5vh,88px)]');
    expect(within(atisButton).queryByRole('group', { name: 'ATIS message' })).not.toBeInTheDocument();
    expect(screen.queryByText(arrivalAtis)).not.toBeInTheDocument();
    const weather = within(atisButton).getByRole('group', { name: 'ATIS weather' });
    expect(weather).toHaveClass('h-[30%]');
    const reminder = within(atisButton).getByRole('note');
    expect(reminder).toHaveTextContent('At First Contact with APP, Stats A/C Type & ATIS Info');
    expect(reminder).toHaveClass('font-normal', 'text-[clamp(12px,2vh,28px)]');
    expect(reminder).toHaveClass('top-[80%]', 'h-[20%]', 'efb-action-bar');
    for (const [label, value] of [['WIND', '16 KT'], ['TEMPERATURE', '11 °C'], ['QNH', '1021 hPa']]) {
      const widget = within(weather).getByRole('group', { name: label });
      expect(widget).toHaveClass('bg-transparent');
      expect(widget).toHaveTextContent(value);
    }
    expect(within(weather).getByRole('img', { name: 'Wind from 360 degrees' })).toHaveStyle({ transform: 'rotate(140deg)' });
    fireEvent.click(within(atisButton).getByRole('group', { name: clickTarget }));
    expect(await screen.findByText('ATIS for SAS676')).toBeInTheDocument();
    expect(screen.getAllByText(arrivalAtis)).toHaveLength(1);
    fireEvent.click(screen.getByRole('button', { name: 'CLOSE' }));
    expect(screen.queryByText('ATIS for SAS676')).not.toBeInTheDocument();
  });

  it.each([
    ['', false, false, 'PDC UNAVAILABLE'],
    ['', true, false, 'REQ PDC'],
    ['REQUESTED', false, false, 'STANDBY'],
    ['', false, true, 'VIEW PDC'],
    ['CONFIRMED', false, false, 'PDC COMPLETE'],
  ])('keeps the PDC label %s/%s/%s on one fitted line', async (state, canSubmit, requiresAction, label) => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase: 'DEPARTURE',
        atis: { code: 'A', text: ['ATIS A'] }, pdc_state: state, pdc_can_submit: canSubmit, pdc_requires_pilot_action: requiresAction,
        capabilities: { pdc: canSubmit, tobt_update: false, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    const text = await screen.findByText(label);
    expect(text).toHaveClass('whitespace-nowrap');
    expect(text).not.toHaveClass('translate-x-[20%]');
    const button = screen.getByRole('button', { name: label });
    expect(button).toContainElement(text);
    expect(button).toHaveClass('min-w-0', 'rounded-none', 'h-full', 'w-full', 'border-[clamp(5px,0.5vw,18px)]', 'border-[#000109]', 'items-center', 'justify-center', '[container-type:inline-size]');
    expect(button.parentElement).toHaveClass('top-[80%]', 'h-[20%]');
    expect(button).toHaveAttribute('type', 'button');
    expect(button).toHaveClass('efb-action-bar');
    if (state === 'REQUESTED' || state === 'CONFIRMED') expect(button).toBeDisabled();
    else expect(button).toBeEnabled();
    expect(screen.queryByAltText('PDC')).not.toBeInTheDocument();
    expect(text).toHaveClass('text-[min(3.5vh,53px,var(--pdc-label-size))]');
    expect(text.style.getPropertyValue('--pdc-label-size')).toBe(`${100 / (label.length * 0.75)}cqw`);
  });

  it('keeps the ATIS and PDC requests separate when ATIS is missing', async () => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase: 'DEPARTURE',
        stand: null, atis: null, capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    expect(await screen.findByText('SAS123')).toBeInTheDocument();
    const tile = screen.getByRole('button', { name: 'REQ PDC' }).closest('[data-efb-tile]');
    expect(tile).toHaveAttribute('data-efb-tile', 'R2');
    expect(screen.getAllByText('REQ ATIS')).toHaveLength(1);
    expect(tile).toHaveTextContent('REQ PDC');
    fireEvent.click(screen.getByText('REQ PDC'));
    expect(await screen.findByText('Current ATIS is required before requesting PDC')).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/api/pdc/request'))).toBe(false);
    expect(screen.getByRole('group', { name: 'Stand' })).not.toHaveClass('bg-[#41826E]');
    expect(screen.getByRole('status', { name: 'Stand state' })).not.toHaveClass('bg-[#41826E]');
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
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveTextContent('SENSED');
    expect(screen.getByRole('group', { name: 'Stand' })).toHaveClass('bg-[#41826E]');
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveClass('bg-[#41826E]');
    expect(screen.getByText('1230')).toBeInTheDocument();
    expect(screen.queryByText('123000')).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'CTOT' })).toHaveTextContent(/^CTOTNIL$/);
    expect(screen.getByRole('group', { name: 'CTOT' })).toHaveClass('bg-[#41826E]');
    expect(screen.queryByText('CONTACT PASSING')).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Departure Freq' })).toHaveTextContent('NIL');
    expect(screen.getByRole('group', { name: 'Departure CTOT' })).toHaveTextContent('NIL');
    expect(screen.getByRole('link', { name: 'Traffic Board' })).toHaveAttribute('href', 'https://flightboard.simfixr.com/?icao=EKCH');
    expect(screen.getByRole('button', { name: 'Downloads' })).toHaveClass('top-[2.5%]', 'h-[60%]', 'w-[30.83%]', 'left-[2.5%]');
    expect(screen.getByRole('link', { name: 'Traffic Board' })).toHaveClass('top-[2.5%]', 'h-[60%]', 'w-[30.83%]', 'left-[66.66%]');
    expect(screen.getByRole('button', { name: 'Downloads' })).toHaveClass('bg-[#011328]', 'text-white', 'efb-action-bar', 'efb-clickspot');
    expect(screen.getByRole('link', { name: 'Traffic Board' })).toHaveClass('bg-[#011328]', 'text-white', 'efb-action-bar', 'efb-clickspot');
    expect(screen.getByRole('button', { name: 'Downloads' })).toHaveTextContent('DOWNLOADS');
    expect(screen.getByRole('link', { name: 'Traffic Board' })).toHaveTextContent('TRAFFIC BOARD');
    const downloadsArrow = screen.getByRole('button', { name: 'Downloads' }).querySelector('img')!;
    const trafficArrow = screen.getByRole('link', { name: 'Traffic Board' }).querySelector('img')!;
    expect(downloadsArrow).toHaveAttribute('src', expect.stringContaining('stand-arrow.png'));
    expect(trafficArrow).toHaveAttribute('src', expect.stringContaining('stand-arrow.png'));
    expect(downloadsArrow).toHaveClass('left-[3%]', 'h-[85%]', 'w-[12%]');
    expect(trafficArrow).toHaveClass('right-[3%]', 'h-[85%]', 'w-[12%]');
    expect(downloadsArrow).toHaveAttribute('alt', '');
    expect(trafficArrow).toHaveAttribute('alt', '');
    expect(screen.queryByAltText('Downloads')).not.toBeInTheDocument();
    expect(screen.queryByAltText('Traffic Board')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Downloads' }));
    expect(screen.getByRole('dialog', { name: 'Downloads' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Download GSX profile' })).toHaveAttribute('href', 'https://www.flightsim.to');
    fireEvent.click(screen.getByRole('button', { name: 'Close downloads' }));

    fireEvent.click(screen.getByAltText('Charts'));
    const chartDialog = screen.getByRole('dialog', { name: 'EKCH' });
    expect(chartDialog).toBeInTheDocument();
    expect(within(chartDialog).getByText('EKCH, runway 22R')).toBeInTheDocument();
    expect(within(chartDialog).getByText('NEXEN2A')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'CLICK TO CLOSE' }));

    fireEvent.click(screen.getByText('REQ PDC'));
    await waitFor(() => {
      const request = fetchMock.mock.calls.find(([input]) => String(input).includes('/api/pdc/request'));
      expect(request).toBeDefined();
      expect(JSON.parse(request![1].body)).toMatchObject({ aircraft_type: 'A320' });
    });
  });

  it.each(['DEPARTURE', 'ARRIVAL'])('applies timing windows and single-line guidance for %s across UTC midnight', async (phase) => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    const clock = vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-10-17T23:53:59.999Z'));
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign: 'SAS123', origin: phase === 'ARRIVAL' ? 'EDDF' : 'EKCH',
        destination: phase === 'ARRIVAL' ? 'EKCH' : 'EDDF', phase,
        tobt: '2345', tsat: '2350', ctot: '0000',
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    await screen.findByText('SAS123');
    const tobt = screen.getByRole('group', { name: 'TOBT' });
    const tsat = screen.getByRole('group', { name: 'TSAT' });
    const ctot = screen.getByRole('group', { name: 'CTOT' });
    for (const text of ['call for start within +/- 5min', 'Expected startup time +/-5min', 'Expect takeoff within -5 to +10']) {
      expect(screen.getByText(text)).toHaveClass('whitespace-nowrap');
    }
    const cases = [
      ['2026-10-17T23:53:59.999Z', 'bg-[#41826E]', false],
      ['2026-10-17T23:54:00.000Z', 'bg-[#41826E]', true],
      ['2026-10-17T23:55:00.000Z', 'bg-[#41826E]', true],
      ['2026-10-17T23:55:00.001Z', 'bg-[#1A475F]', true],
      ['2026-10-17T23:56:00.000Z', 'bg-[#1A475F]', true],
      ['2026-10-17T23:56:00.001Z', 'bg-[#B63F3F]', true],
      ['2026-10-18T00:00:00.000Z', 'bg-[#B63F3F]', true],
      ['2026-10-18T00:11:00.000Z', 'bg-[#B63F3F]', true],
      ['2026-10-18T00:11:00.001Z', 'bg-[#B63F3F]', false],
    ] as const;
    for (const [now, tsatColor, ctotGreen] of cases) {
      clock.mockReturnValue(Date.parse(now));
      await act(async () => { await vi.advanceTimersByTimeAsync(500); });
      expect(tsat).toHaveClass(tsatColor);
      expect(ctot).toHaveClass(ctotGreen ? 'bg-[#41826E]' : 'bg-[#1A475F]');
      expect(tobt).toHaveClass(tsatColor === 'bg-[#B63F3F]' ? 'bg-[#B63F3F]' : 'bg-[#1A475F]');
    }
  });

  it('hides guidance for missing TSAT and CTOT while retaining the green CTOT NIL fill', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase: 'DEPARTURE', tobt: '1200',
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    await screen.findByText('SAS123');
    expect(screen.getByText('call for start within +/- 5min')).toBeInTheDocument();
    expect(screen.queryByText('Expected startup time +/-5min')).not.toBeInTheDocument();
    expect(screen.queryByText('Expect takeoff within -5 to +10')).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'TSAT' })).toHaveClass('bg-transparent');
    expect(screen.getByRole('group', { name: 'CTOT' })).toHaveClass('bg-[#41826E]');
  });

  it('shows departure CDM timings and submits a TOBT update using the active account', async () => {
    let updated = false;
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      if (url.includes('/api/efb/tobt')) {
        expect(init?.method).toBe('PUT');
        expect(JSON.parse(String(init?.body))).toEqual({ tobt: '2040' });
        updated = true;
        return jsonResponse({ callsign: 'SAS123', tobt: '2040' });
      }
      return jsonResponse({
        callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase: 'DEPARTURE',
        stand: 'A12', stand_state: 'SENSED', tobt: updated ? '2040' : '2010', tsat: '201500', ctot: '2030',
        atis: { callsign: 'EKCH_D_ATIS', code: 'D', text: ['EKCH DEP ATIS D', 'RWY IN USE 22R'], stale: false },
        capabilities: { pdc: false, tobt_update: true, stand_reassignment: false },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    expect(await screen.findByText('SAS123')).toBeInTheDocument();
    expect(screen.queryByLabelText('Development callsign')).not.toBeInTheDocument();
    expect(screen.getByText('2010')).toBeInTheDocument();
    expect(screen.getByText('2015')).toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'CTOT' })).toHaveTextContent('2030');
    expect(screen.getByRole('group', { name: 'CTOT' })).toHaveClass('bg-[#1A475F]');
    expect(screen.getByRole('group', { name: 'CTOT' }).closest('[data-efb-tile]')).toHaveAttribute('data-efb-tile', 'M2');
    expect(screen.getByAltText('TOBT stand guidance').closest('[data-efb-tile]')).toHaveAttribute('data-efb-tile', 'M2');
    expect(screen.getByText('2010').closest('[data-efb-tile]')).toHaveAttribute('data-efb-tile', 'M2');
    expect(screen.getByAltText('Departure Info').closest('[data-efb-tile]')).toHaveAttribute('data-efb-tile', 'M3');
    expect(screen.getByAltText('Departure Info').closest('[data-efb-tile]')).toHaveTextContent('2030');
    expect(screen.getByAltText('Charts').closest('[data-efb-tile]')).toHaveAttribute('data-efb-tile', 'L3');
    expect(screen.getByText('ATIS D').closest('[data-efb-tile]')).toHaveAttribute('data-efb-tile', 'R2');
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    const menu = screen.getByRole('dialog', { name: 'TOBT' });
    expect(menu).toHaveClass('left-[34.58%]');
    expect(menu).toHaveClass('rounded-none', 'border-0', 'bg-[#000109]', 'w-[30.83%]');
    const timingTile = screen.getByRole('group', { name: 'TOBT' }).closest<HTMLElement>('[data-efb-tile]')!;
    expect(menu.style.top).toBe(timingTile.style.top);
    expect(menu.style.height).toBe(timingTile.style.height);
    fireEvent.change(within(menu).getByLabelText('New TOBT (UTC)'), { target: { value: '2040Z' } });
    fireEvent.click(within(menu).getByRole('button', { name: 'UPDATE TOBT' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'TOBT' })).not.toBeInTheDocument());
    expect(screen.getByText('2040')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'TOBT CONFIRMED' }));
    expect(screen.getByRole('dialog', { name: 'TOBT' })).toBeInTheDocument();
    fireEvent.click(within(screen.getByRole('dialog', { name: 'TOBT' })).getByRole('button', { name: 'CANCEL' }));
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveTextContent('SENSED');
    fireEvent.click(screen.getByText('ATIS D'));
    expect(await screen.findByText('ATIS for SAS123')).toBeInTheDocument();
    expect(screen.getByText(/EKCH DEP ATIS D/)).toBeInTheDocument();
  });

  it.each([
    ['22R', 'FL70', 'GOLGA', '120.255'],
    ['12', '4000ft', 'NEXEN2A', '124.980'],
    ['30', '4000ft', 'VEDAR1A', '120.255'],
  ])('shows published departure climb and SID frequency for runway %s', async (runway, climb, sid, frequency) => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS99X' });
      return jsonResponse({
        callsign: 'SAS99X', origin: 'EKCH', destination: 'EKYT', phase: 'DEPARTURE',
        runway, sid, initial_climb: climb, cleared_altitude: null, departure_frequency: frequency,
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    expect(await screen.findByText(climb)).toBeInTheDocument();
    const tile = screen.getByAltText('Departure Info').closest('[data-efb-tile]');
    expect(tile).toHaveAttribute('data-efb-tile', 'M3');
    expect(tile).toHaveTextContent(runway);
    expect(tile).toHaveTextContent(sid);
    expect(tile).toHaveTextContent(frequency);
    for (const label of ['Initial Climb', 'Departure Freq', 'Departure CTOT', 'SID']) {
      const group = screen.getByRole('group', { name: label });
      expect(tile).toContainElement(group);
      expect(group.firstElementChild).toHaveClass('border-b', 'border-[#38536E]', 'text-[#B8D9F0]');
    }
    expect(screen.getByRole('group', { name: 'Departure runway' })).toHaveTextContent(runway);
    expect(screen.getByRole('group', { name: 'Departure runway' })).toHaveClass('left-[40%]', 'w-[18%]', 'justify-center');
    expect(screen.getByAltText('Departure Info').parentElement).toHaveClass('left-[34%]', '-translate-y-[4%]');
    const departureFrequency = screen.getByRole('group', { name: 'Departure Freq' });
    expect(within(departureFrequency).getByAltText('Headset')).toBeInTheDocument();
    expect(departureFrequency).toHaveTextContent('Contact AUTOMATICALLYafter departure');
    const automatically = within(departureFrequency).getByText('AUTOMATICALLY');
    expect(automatically.parentElement).toHaveClass('text-[clamp(12px,1.8vh,32px)]');
    expect(automatically.tagName).toBe('STRONG');
    expect(automatically).toHaveClass('underline');
    expect(automatically).not.toHaveClass('text-red-500');
    expect(tile).not.toHaveTextContent('FL40');
  });

  it('opens the arrival briefing with the selected STAR and runway guidance', async () => {
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: false, online_callsign: null });
      return jsonResponse({
        callsign: 'SAS790', aircraft_type: 'A320', origin: 'ESSA', destination: 'EKCH', phase: 'ARRIVAL',
        stand: 'A12', runway: '22L', sid: null, star: 'TUDLO4C', tobt: '1200', eobt: '1200', ctot: '1230', tsat: '1215',
        pdc_state: '', pdc_requires_pilot_action: false, pdc_available: false, pdc_can_submit: false,
        atis: { code: 'A', text: ['ATIS A'] },
        capabilities: { pdc: false, tobt_update: true, stand_reassignment: false },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);

    fireEvent.change(await screen.findByLabelText('Development callsign'), { target: { value: 'sas790' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));

    expect(await screen.findByText('TUDLO4C')).toBeInTheDocument();
    expect(screen.getByText('TOBT')).toBeInTheDocument();
    expect(screen.getByText('TSAT')).toBeInTheDocument();
    const tobtTile = screen.getByRole('group', { name: 'TOBT' });
    const tsatTile = screen.getByRole('group', { name: 'TSAT' });
    const ctotTile = screen.getByRole('group', { name: 'CTOT' });
    expect(tobtTile).toHaveClass('left-0', 'w-1/3');
    expect(tsatTile).toHaveClass('left-1/3', 'w-1/3');
    expect(ctotTile).toHaveClass('left-2/3', 'w-1/3');
    expect(ctotTile).toHaveTextContent('1230');
    expect(within(tobtTile).queryByText('Confirmed')).not.toBeInTheDocument();
    expect(screen.queryByText('UPDATE UNAVAILABLE')).not.toBeInTheDocument();
    const updateTobtTile = screen.getByText('UPDATE TOBT');
    expect(updateTobtTile).toHaveClass('efb-action-bar', 'top-[80%]', 'h-[20%]', 'border-[#000109]', 'bg-[#1A475F]', 'text-[clamp(14px,3.5vh,53px)]');
    expect(screen.getByAltText('TOBT stand guidance')).toHaveClass('h-full', 'w-full', 'scale-[1.15]', 'object-contain');
    fireEvent.click(tobtTile);
    const tobtDialog = screen.getByRole('dialog', { name: 'TOBT' });
    expect(tobtDialog).toHaveClass('left-[2.5%]');
    expect(within(tobtDialog).getByRole('button', { name: 'UPDATE TOBT' })).toBeInTheDocument();
    fireEvent.click(within(tobtDialog).getByRole('button', { name: 'CANCEL' }));
    expect(screen.queryByRole('dialog', { name: 'TOBT' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Open arrival briefing' })).toContainElement(screen.getByAltText('Arrival briefing'));
    const arrivalAtis = screen.getByRole('button', { name: 'Open ATIS' });
    expect(arrivalAtis).toHaveClass('bg-[#000109]');
    expect(arrivalAtis).toHaveClass('border-[#000109]');
    const atisIdentifier = within(arrivalAtis).getByRole('group', { name: 'ATIS identifier' });
    expect(atisIdentifier).toHaveClass('top-0', 'h-1/2', 'w-full', 'bg-[#1A475F]');
    expect(atisIdentifier).toHaveClass('efb-action-bar');
    expect(atisIdentifier).toHaveTextContent('ATIS A');
    expect(atisIdentifier).toHaveClass('flex-col', 'items-center');
    expect(within(atisIdentifier).getByText('ATIS A')).toHaveClass('w-full', 'text-center');
    expect(within(atisIdentifier).getByAltText('ATIS icon').parentElement).toHaveClass('w-full');
    expect(within(arrivalAtis).queryByRole('group', { name: 'ATIS message' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Open arrival briefing' }));
    expect(screen.getByRole('dialog', { name: 'Arrival briefing' })).toBeInTheDocument();
    expect(screen.getByText('Welcome to Copenhagen')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'CLICK TO CLOSE' }));
    fireEvent.click(screen.getByRole('button', { name: 'Open ATIS' }));
    expect(await screen.findByText('ATIS for SAS790')).toBeInTheDocument();
  });

  it('shows API failures as retryable errors instead of pretending there is no flight', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => jsonResponse({ error: 'pilot lookup unavailable' }, 503)));
    render(<EFBPage />);

    expect(await screen.findByRole('button', { name: 'CLICK TO RETRY' })).toBeInTheDocument();
    expect(screen.getByText('pilot lookup unavailable')).toBeInTheDocument();
    expect(screen.queryByText('NO FLIGHTPLAN')).not.toBeInTheDocument();
  });

  it('previews the supplied simulated arrival without calling flight or controller APIs', async () => {
    const fetchMock = vi.fn().mockImplementation(previewResponse);
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);

    fireEvent.click(screen.getByRole('button', { name: 'PREVIEW ARRIVAL' }));
    expect(screen.getByRole('form', { name: 'Simulated arrival' })).toBeInTheDocument();
    expect(screen.getByLabelText('Callsign')).toHaveValue('SAS676');
    expect(screen.getByLabelText('Origin ICAO')).toHaveValue('EDDF');
    expect(screen.getByLabelText('Destination ICAO')).toHaveValue('EKCH');
    expect(screen.getByLabelText('Aircraft type')).toHaveValue('A320');
    expect(screen.getByLabelText('Cruise flight level')).toHaveValue('340');
    fireEvent.change(screen.getByLabelText('Stand'), { target: { value: 'B4' } });
    fireEvent.change(screen.getByLabelText('TOBT (HHMM UTC)'), { target: { value: '1935' } });
    fireEvent.change(screen.getByLabelText('TSAT (HHMM UTC)'), { target: { value: '1940Z' } });
    fireEvent.change(screen.getByLabelText('CTOT (HHMM UTC)'), { target: { value: '1950' } });
    expect(screen.getByLabelText('Route')).toHaveValue('MARUN Y150 TOLGI SAS MAKEL N851 GESKA Z711 MONAK');
    fireEvent.change(screen.getByLabelText('Arrival runway'), { target: { value: '22L' } });
    fireEvent.change(screen.getByLabelText('Arrival estimate (HHMM)'), { target: { value: '1230' } });
    fireEvent.change(screen.getByLabelText('STAR'), { target: { value: 'MONAK5C' } });
    fireEvent.change(screen.getByLabelText('Published holding fix'), { target: { value: 'MONAK' } });
    fireEvent.change(screen.getByLabelText('Holding detail'), { target: { value: 'RIGHT 1 MIN' } });
    fireEvent.change(screen.getByLabelText('Published heading'), { target: { value: '37' } });

    fireEvent.click(screen.getByRole('button', { name: 'SHOW ARRIVAL' }));
    await screen.findByText('ATIS C');
    const preview = screen.getByRole('status', { name: 'Simulated arrival preview' });
    expect(within(preview).getByText(/SAS676 · EDDF–EKCH · A320 · FL340/)).toBeInTheDocument();
    expect(within(preview).getByText('MARUN Y150 TOLGI SAS MAKEL N851 GESKA Z711 MONAK')).toBeInTheDocument();
    expect(screen.getByText('1935')).toBeInTheDocument();
    expect(screen.getByText('1940')).toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'CTOT' })).toHaveTextContent('1950');
    const previewAtis = screen.getByRole('button', { name: 'Open ATIS' });
    expect(screen.getByRole('status', { name: 'Stand state' })).not.toHaveClass('efb-clickspot');
    const atisIdentifier = within(previewAtis).getByRole('group', { name: 'ATIS identifier' });
    expect(atisIdentifier).toHaveTextContent('ATIS C');
    expect(within(atisIdentifier).getByAltText('ATIS icon')).toBeInTheDocument();
    expect(atisIdentifier).toHaveClass('flex-col', 'items-center');
    expect(within(atisIdentifier).getByText('ATIS C')).toHaveClass('w-full', 'text-center', 'text-[clamp(14px,5.5vh,88px)]');
    expect(within(previewAtis).queryByRole('group', { name: 'ATIS message' })).not.toBeInTheDocument();
    expect(screen.queryByText(/EKCH ARR ATIS C/)).not.toBeInTheDocument();
    expect(await screen.findByRole('button', { name: 'Open arrival briefing' })).toBeInTheDocument();
    const arrivalApproach = screen.getByRole('group', { name: 'Expected APP' });
    expect(arrivalApproach).toHaveTextContent('ILS 22L');
    expect(within(arrivalApproach).getByText('EXPECTED APP')).toBeInTheDocument();
    expect(arrivalApproach).toHaveClass('right-[2%]');
    expect(screen.getByAltText('Arrival Info').parentElement).toHaveClass('left-[34%]', '-translate-y-[4%]');
    const expectedStar = screen.getByRole('group', { name: 'Expected STAR' });
    expect(expectedStar).toHaveTextContent('EXPECTED STAR');
    expect(expectedStar).toHaveTextContent('MONAK5C');
    expect(expectedStar).toHaveClass('left-[2%]');
    expect(within(expectedStar).getByText('EXPECTED STAR')).toHaveClass('w-[86%]');
    expect(within(arrivalApproach).getByText('EXPECTED APP')).toHaveClass('w-[108%]');
    expect(screen.queryByText('EAT')).not.toBeInTheDocument();
    const publishedHolding = screen.getByRole('group', { name: 'Published holding' });
    expect(within(publishedHolding).getByText('PUBLISHED HOLDING')).toHaveClass('whitespace-nowrap');
    expect(within(publishedHolding).getByText('PUBLISHED HOLDING')).toHaveClass('w-[86%]');
    expect(within(publishedHolding).getByText('PUBLISHED HOLDING')).toHaveClass('text-[clamp(10px,1.7vh,30px)]');
    expect(within(publishedHolding).getByText('MONAK')).toHaveClass('text-[clamp(14px,3.5vh,53px)]');
    expect(within(publishedHolding).getByText('RIGHT 1 MIN')).toHaveClass('text-[clamp(14px,2.5vh,53px)]');
    expect(within(publishedHolding).getByText('MONAK')).toHaveClass('top-[16%]', 'left-[35%]');
    expect(within(publishedHolding).getByText('RIGHT 1 MIN')).toHaveClass('top-[38%]', 'left-[35%]');
    expect(screen.getByText('Maintain HDG037').parentElement).toHaveClass('top-[38%]', 'right-0');
    expect(screen.getByText('Maintain HDG037')).toHaveClass('underline', 'underline-offset-4');
    const afterPassing = screen.getByRole('group', { name: 'After passing' });
    expect(afterPassing).toHaveClass('w-[108%]');
    expect(publishedHolding).toContainElement(screen.getByAltText('Holding pattern'));
    expect(screen.getByAltText('Holding pattern')).toHaveClass('h-[46%]', 'w-[30%]');
    expect(publishedHolding.parentElement).toContainElement(screen.getByAltText('Arrival Info'));
    expect(screen.getByRole('group', { name: 'After passing' })).toBeInTheDocument();

    expect(fetchMock.mock.calls.every(([_input]) => !String(_input).includes('/api/efb/flight'))).toBe(true);
    expect(fetchMock.mock.calls.every(([_input]) => !String(_input).includes('/api/pdc/'))).toBe(true);
    expect(fetchMock.mock.calls.every(([_input, init]) => !init?.method || init.method === 'GET')).toBe(true);
  });

  it('validates simulated TOBT, TSAT, and CTOT as HHMM UTC times', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => jsonResponse({ error: 'EFB API unavailable' }, 503)));
    render(<EFBPage />);

    fireEvent.click(screen.getByRole('button', { name: 'PREVIEW ARRIVAL' }));
    fireEvent.change(screen.getByLabelText('CTOT (HHMM UTC)'), { target: { value: '2460' } });
    fireEvent.click(screen.getByRole('button', { name: 'SHOW ARRIVAL' }));

    expect(screen.getByRole('alert')).toHaveTextContent('TOBT, TSAT, and CTOT must be valid HHMM UTC times');
    expect(screen.getByRole('form', { name: 'Simulated arrival' })).toBeInTheDocument();
  });

  it.each([
    { stand: 'B4', stage: 'ESTIMATED', expected: 'B4', state: 'ESTIMATED' },
    { stand: 'C27', stage: 'ASSIGNED', expected: 'C27', state: 'ASSIGNED' },
    { stand: 'D1', stage: 'CONFIRMED', expected: 'D1', state: 'CONFIRMED' },
    { stand: null, stage: null, expected: 'TBA', state: 'TBA' },
    { stand: 'B4', stage: 'RELEASED', expected: 'TBA', state: 'TBA' },
    { stand: '', stage: 'ASSIGNED', expected: 'TBA', state: 'TBA' },
  ])('shows the FlightStrips stand $expected with state $state', async ({ stand, stage, expected, state }) => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: false });
      return jsonResponse({
        callsign: 'SAS123', origin: 'EDDF', destination: 'EKCH', phase: 'ARRIVAL', stand, stand_state: stage,
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    fireEvent.change(await screen.findByLabelText('Development callsign'), { target: { value: 'SAS123' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));
    expect(await screen.findByRole('group', { name: 'Stand' })).toHaveTextContent(expected);
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveTextContent(state);
  });

  it('submits a manual stand selection with its version and displays the assigned result', async () => {
    let assigned = false;
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes('/api/efb/me')) return jsonResponse({ live_mode: false });
      if (url.includes('/api/efb/stands')) return jsonResponse({ stands: [{ stand: 'B4', available: true }, { stand: 'C27', available: true }] });
      if (url.endsWith('/api/efb/stand')) {
        expect(init?.method).toBe('PUT');
        expect(JSON.parse(String(init?.body))).toEqual({ callsign: 'SAS123', stand: 'C27', version: 3 });
        assigned = true;
        return jsonResponse({ callsign: 'SAS123', stand: 'C27', stand_state: 'ASSIGNED', version: 4 });
      }
      return jsonResponse({
        callsign: 'SAS123', origin: 'EDDF', destination: 'EKCH', phase: 'ARRIVAL',
        stand: assigned ? 'C27' : 'B4', stand_state: assigned ? 'ASSIGNED' : 'ESTIMATED', stand_version: assigned ? 4 : 3,
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: true },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    fireEvent.change(await screen.findByLabelText('Development callsign'), { target: { value: 'SAS123' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));
    fireEvent.click(await screen.findByRole('group', { name: 'Stand' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Stand C27' })).toBeEnabled());
    fireEvent.click(screen.getByRole('button', { name: 'Stand C27' }));
    fireEvent.click(screen.getByRole('button', { name: 'REQUEST NEW STAND' }));
    await waitFor(() => expect(screen.queryByRole('button', { name: 'CLICK TO CANCEL' })).not.toBeInTheDocument());
    expect(assigned).toBe(true);
    expect(screen.getByRole('group', { name: 'Stand' })).toHaveTextContent('C27');
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveTextContent('ASSIGNED');
  });

  it('submits an arrival TOBT and refreshes the vIFF timing snapshot', async () => {
    let updated = false;
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).includes('/api/efb/me')) return jsonResponse({ live_mode: false, online_callsign: null });
      if (String(input).includes('/api/efb/tobt')) {
        expect(init?.method).toBe('PUT');
        expect(JSON.parse(String(init?.body))).toEqual({ callsign: 'SAS123', tobt: '2010' });
        updated = true;
        return jsonResponse({ callsign: 'SAS123', tobt: '2010' });
      }
      return jsonResponse({
        callsign: 'SAS123', aircraft_type: 'A320', origin: 'EDDF', destination: 'EKCH', phase: 'ARRIVAL',
        tobt: updated ? '2010' : '1935', tsat: '1940', ctot: '1950',
        capabilities: { pdc: false, tobt_update: true, stand_reassignment: false },
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    fireEvent.change(await screen.findByLabelText('Development callsign'), { target: { value: 'SAS123' } });
    fireEvent.click(screen.getByRole('button', { name: 'LOAD' }));
    expect(await screen.findByText('1935')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    const dialog = screen.getByRole('dialog', { name: 'TOBT' });
    fireEvent.change(within(dialog).getByLabelText('New TOBT (UTC)'), { target: { value: '2010Z' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'UPDATE TOBT' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'TOBT' })).not.toBeInTheDocument());
    expect(updated).toBe(true);
    expect(screen.getByRole('group', { name: 'TOBT' })).toHaveTextContent('2010');
    expect(screen.getByRole('group', { name: 'TOBT' })).toHaveClass('bg-[#41826E]');
    expect(screen.getByRole('button', { name: 'TOBT CONFIRMED' })).toHaveClass('bg-[#41826E]');
  });

  it.each(['ARRIVAL', 'DEPARTURE'])('shows NIL on green without CTOT and does not confirm a failed %s TOBT update', async (phase) => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      if (String(input).includes('/tobt')) return jsonResponse({ error: 'TOBT update unavailable' }, 503);
      return jsonResponse({
        callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase, tobt: '2010', ctot: null,
        capabilities: { pdc: false, tobt_update: true, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    await screen.findByText('SAS123');
    const ctot = screen.getByRole('group', { name: 'CTOT' });
    expect(ctot).toHaveTextContent(/^CTOTNIL$/);
    expect(ctot).toHaveClass('bg-[#41826E]');
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    const dialog = screen.getByRole('dialog', { name: 'TOBT' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'UPDATE TOBT' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('TOBT update unavailable');
    expect(screen.queryByRole('button', { name: 'TOBT CONFIRMED' })).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'TOBT' })).not.toHaveClass('bg-[#41826E]');
    expect(screen.getByRole('group', { name: 'TOBT' })).toHaveTextContent('2010');
  });

  it('allows previewing the TOBT picker without sending a write', () => {
    const fetchMock = vi.fn().mockImplementation(() => jsonResponse({ error: 'EFB API unavailable' }, 503));
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    fireEvent.click(screen.getByRole('button', { name: 'PREVIEW ARRIVAL' }));
    fireEvent.click(screen.getByRole('checkbox', { name: 'On ground' }));
    fireEvent.click(screen.getByRole('button', { name: 'SHOW ARRIVAL' }));
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    const dialog = screen.getByRole('dialog', { name: 'TOBT' });
    expect(within(dialog).getByRole('button', { name: 'UPDATE TOBT' })).toBeDisabled();
    expect(fetchMock.mock.calls.every(([_input, init]) => !init?.method || init.method === 'GET')).toBe(true);
  });

  it.each(['ARRIVAL', 'DEPARTURE'])('leaves unfilled %s timing fields flat', async (phase) => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign: 'SAS123', origin: 'EDDF', destination: 'EKCH', phase,
        capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    await screen.findByText('SAS123');
    expect(screen.getByRole('group', { name: 'TOBT' })).not.toHaveClass('efb-action-bar');
    expect(screen.getByRole('group', { name: 'TSAT' })).not.toHaveClass('efb-action-bar');
    expect(screen.getByRole('group', { name: 'CTOT' })).toHaveClass('efb-action-bar', 'bg-[#41826E]');
  });

  it('greys only preview arrival timing while keeping other tiles usable in air and on ground', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(previewResponse));
    render(<EFBPage />);
    fireEvent.click(screen.getByRole('button', { name: 'PREVIEW ARRIVAL' }));
    expect(screen.getByRole('checkbox', { name: 'On ground' })).not.toBeChecked();
    fireEvent.click(screen.getByRole('button', { name: 'SHOW ARRIVAL' }));

    const tobt = screen.getByRole('group', { name: 'TOBT' });
    expect(tobt.closest('[inert]')).toBeNull();
    for (const name of ['TOBT', 'TSAT', 'CTOT']) {
      expect(screen.getByRole('group', { name })).toHaveClass('bg-[#2E343D]');
    }
    expect(screen.getByRole('button', { name: 'INHIBITED IN AIR' })).toBeDisabled();
    fireEvent.click(tobt);
    expect(screen.queryByText('TOBT updates are currently unavailable')).not.toBeInTheDocument();
    const onGround = screen.getByRole('checkbox', { name: 'On ground' });
    fireEvent.click(screen.getByRole('button', { name: 'Open arrival briefing' }));
    expect(screen.getByRole('dialog', { name: 'Arrival briefing' })).toBeInTheDocument();

    fireEvent.click(onGround);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByText('INHIBITED IN AIR')).not.toBeInTheDocument();
    expect(screen.queryByText(/inhibited on ground/i)).not.toBeInTheDocument();
    expect(tobt.closest('[inert]')).toBeNull();
    expect(screen.getByRole('button', { name: 'UPDATE TOBT' })).toBeEnabled();
    fireEvent.click(screen.getByRole('button', { name: 'Open arrival briefing' }));
    expect(screen.getByRole('dialog', { name: 'Arrival briefing' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'CLICK TO CLOSE' }));
    expect(screen.queryByText('Stand reassignment is currently unavailable')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('checkbox', { name: 'On ground' }));
    expect(screen.getByRole('button', { name: 'INHIBITED IN AIR' })).toBeDisabled();
    await screen.findByText('ATIS C');
    fireEvent.click(screen.getByRole('button', { name: 'Open ATIS' }));
    expect(await screen.findByText('ATIS for SAS676')).toBeInTheDocument();
  });

  it.each([
    ['ARRIVAL', 0, false],
    ['ARRIVAL', 60, false],
    ['ARRIVAL', 60.1, true],
    ['ARRIVAL', 450, true],
    ['ARRIVAL', null, false],
    ['DEPARTURE', 450, false],
  ])('uses the exact arrival speed threshold for %s at %s knots', async (phase, groundspeed, inhibited) => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign: 'SAS123', origin: 'EDDF', destination: 'EKCH', phase, groundspeed,
        tobt: '1200', tsat: '1210', ctot: '1220',
        capabilities: { pdc: false, tobt_update: true, stand_reassignment: true },
      });
    }));
    render(<EFBPage />);
    await screen.findByText('SAS123');
    const button = screen.getByRole('button', { name: inhibited ? 'INHIBITED IN AIR' : 'UPDATE TOBT' });
    expect(button).toHaveProperty('disabled', inhibited);
    for (const name of ['TOBT', 'TSAT', 'CTOT']) {
      const timing = screen.getByRole('group', { name });
      expect(timing.closest('[inert]')).toBeNull();
      expect(timing).toHaveClass('efb-action-bar');
      if (inhibited) expect(timing).toHaveClass('bg-[#2E343D]');
      else expect(timing).not.toHaveClass('bg-[#2E343D]');
    }
    fireEvent.click(screen.getByRole('group', { name: 'TOBT' }));
    expect(screen.queryByRole('dialog', { name: 'TOBT' }) !== null).toBe(!inhibited);
  });

  it('closes an open timing dialog when a refreshed arrival exceeds 60 knots and unlocks when it slows', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    let groundspeed = 60;
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      if (String(input).includes('/me')) return jsonResponse({ live_mode: true, online_callsign: 'SAS123' });
      return jsonResponse({
        callsign: 'SAS123', origin: 'EDDF', destination: 'EKCH', phase: 'ARRIVAL', groundspeed,
        capabilities: { pdc: false, tobt_update: true, stand_reassignment: false },
      });
    }));
    render(<EFBPage />);
    await screen.findByText('SAS123');
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    expect(screen.getByRole('dialog', { name: 'TOBT' })).toBeInTheDocument();
    groundspeed = 61;
    await act(async () => { await vi.advanceTimersByTimeAsync(15_000); });
    expect(screen.queryByRole('dialog', { name: 'TOBT' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'INHIBITED IN AIR' })).toBeDisabled();
    groundspeed = 60;
    await act(async () => { await vi.advanceTimersByTimeAsync(15_000); });
    expect(screen.getByRole('button', { name: 'UPDATE TOBT' })).toBeEnabled();
  });

  it('refreshes live preview ATIS and clears it when the station goes offline', async () => {
    vi.useFakeTimers();
    try {
      let online = true;
      vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => (
        !online && String(input).includes('afv-atis-data.json') ? jsonResponse([]) : previewResponse(input)
      )));
      render(<EFBPage />);
      fireEvent.click(screen.getByRole('button', { name: 'PREVIEW ARRIVAL' }));
      await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'SHOW ARRIVAL' })); });
      expect(screen.getByRole('group', { name: 'ATIS identifier' })).toHaveTextContent('ATIS C');
      expect(screen.getByRole('group', { name: 'ATIS identifier' })).toHaveClass('bg-[#1A475F]');
      fireEvent.click(screen.getByRole('button', { name: 'Open ATIS' }));
      expect(screen.getByText('ATIS for SAS676')).toBeInTheDocument();
      expect(screen.getAllByText(/EKCH ARR ATIS C/)).toHaveLength(1);
      fireEvent.click(screen.getByRole('button', { name: 'CLOSE' }));
      expect(screen.getByRole('group', { name: 'ATIS identifier' })).toHaveClass('bg-[#41826E]');
      await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
      expect(screen.getByRole('group', { name: 'ATIS identifier' })).toHaveClass('bg-[#41826E]');
      online = false;
      await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
      expect(screen.getByRole('group', { name: 'ATIS weather' })).toHaveTextContent('N/A');
      expect(screen.queryByText('ATIS C')).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole('button', { name: 'Open ATIS' }));
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      expect(screen.getByText('EKCH_A_ATIS is offline')).toBeInTheDocument();
      online = true;
      await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
      expect(screen.getByRole('group', { name: 'ATIS identifier' })).toHaveTextContent('ATIS C');
    } finally {
      vi.useRealTimers();
    }
  });

  it('shows TBA for an empty preview stand without losing the current ATIS', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(previewResponse));
    render(<EFBPage />);
    fireEvent.click(screen.getByRole('button', { name: 'PREVIEW ARRIVAL' }));
    fireEvent.click(screen.getByRole('button', { name: 'SHOW ARRIVAL' }));
    await screen.findByText('ATIS C');
    fireEvent.click(screen.getByRole('button', { name: 'EDIT' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Stand' }), { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'SHOW ARRIVAL' }));
    expect(screen.getByRole('group', { name: 'Stand' })).toHaveTextContent('TBA');
    expect(screen.getByRole('status', { name: 'Stand state' })).toHaveTextContent('TBA');
    expect(screen.getByRole('group', { name: 'ATIS identifier' })).toHaveTextContent('ATIS C');
  });

  it('restores test-flight fields and ground state after remounting', () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => jsonResponse({ error: 'EFB API unavailable' }, 503)));
    const first = render(<EFBPage />);
    fireEvent.click(screen.getByRole('button', { name: 'PREVIEW ARRIVAL' }));
    fireEvent.change(screen.getByLabelText('Stand'), { target: { value: 'B4' } });
    fireEvent.change(screen.getByLabelText('TOBT (HHMM UTC)'), { target: { value: '1935' } });
    fireEvent.change(screen.getByLabelText('STAR'), { target: { value: 'MONAK5C' } });
    fireEvent.click(screen.getByRole('checkbox', { name: 'On ground' }));
    first.unmount();
    render(<EFBPage />);
    fireEvent.click(screen.getByRole('button', { name: 'PREVIEW ARRIVAL' }));
    expect(screen.getByLabelText('Stand')).toHaveValue('B4');
    expect(screen.getByLabelText('TOBT (HHMM UTC)')).toHaveValue('1935');
    expect(screen.getByLabelText('STAR')).toHaveValue('MONAK5C');
    expect(screen.getByRole('checkbox', { name: 'On ground' })).toBeChecked();
  });

  it('validates simulated arrival airport codes before opening the preview', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => jsonResponse({ error: 'EFB API unavailable' }, 503)));
    render(<EFBPage />);

    fireEvent.click(screen.getByRole('button', { name: 'PREVIEW ARRIVAL' }));
    fireEvent.change(screen.getByLabelText('Destination ICAO'), { target: { value: 'EKC' } });
    fireEvent.click(screen.getByRole('button', { name: 'SHOW ARRIVAL' }));

    expect(screen.getByRole('alert')).toHaveTextContent('valid four-letter origin and destination ICAO codes');
    expect(screen.getByRole('form', { name: 'Simulated arrival' })).toBeInTheDocument();
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
    expect(screen.getByText('REQ PDC')).toBeInTheDocument();
    fireEvent.click(screen.getByText('REQ PDC'));
    expect(await screen.findByText('Current ATIS is required before requesting PDC')).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/api/pdc/request'))).toBe(false);
  });
});
