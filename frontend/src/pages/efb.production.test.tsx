import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import EFBPage from './efb';

vi.mock('@/lib/local-development', () => ({ isLocalDevelopment: false }));
vi.mock('@auth0/auth0-react', () => {
  const auth = { getAccessTokenSilently: vi.fn().mockResolvedValue('token') };
  return { useAuth0: () => auth };
});

describe('production EFB', () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();
  });
  afterEach(() => { vi.restoreAllMocks(); });

  it('uses the authenticated flight without exposing preview, callsign lookup or training controls', async () => {
    window.sessionStorage.setItem('efb-training-session', '20');
    window.sessionStorage.setItem('efb-test-callsign', 'OTHER');
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(
      String(input).includes('/api/efb/me')
        ? { live_mode: true, callsign_locked: true, online_callsign: 'SAS123', training_available: false }
        : { callsign: 'SAS123', origin: 'EDDF', destination: 'EKCH', phase: 'ARRIVAL',
          star: 'TUDLO', runway: '22L', terminal_fix: 'ABEGI', published_heading: 37,
          published_holding_fix: 'LUGAS', published_holding_detail: '077/LEFT', arrival_guidance_expected: true,
          capabilities: { pdc: false, tobt_update: false, stand_reassignment: false } }
    ), { status: 200 })));
    vi.stubGlobal('fetch', fetchMock);
    render(<EFBPage />);
    await screen.findByText('SAS123');
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: 'EFB data source' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'PREVIEW ARRIVAL' })).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Expected STAR' })).toHaveTextContent('TUDLO');
    expect(screen.getByRole('group', { name: 'Expected STAR' })).toHaveTextContent('(EST)');
    expect(screen.getByText('Maintain HDG037')).toBeInTheDocument();
    await waitFor(() => expect(window.sessionStorage.getItem('efb-training-session')).toBeNull());
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('training_session=') || String(url).includes('callsign='))).toBe(false);
    fireEvent.click(screen.getByRole('button', { name: 'Open arrival briefing' }));
    expect(screen.getByText(/NOT AN ATC CLEARANCE/)).toBeInTheDocument();
  });

  it('falls back to the METAR when no ATIS is online', async () => {
    const metar = 'EKCH 191150Z 24012KT 9999 FEW030 08/04 Q1012 NOSIG';
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(
      String(input).includes('/api/efb/me')
        ? { live_mode: true, callsign_locked: true, online_callsign: 'SAS123', training_available: false }
        : { callsign: 'SAS123', origin: 'EKCH', destination: 'EDDF', phase: 'DEPARTURE', atis: null, metar,
          pdc_state: '', pdc_available: false, pdc_can_submit: false, pdc_requires_pilot_action: false,
          capabilities: { pdc: false, tobt_update: false, stand_reassignment: false } }
    ), { status: 200 }))));
    render(<EFBPage />);
    await screen.findByText('SAS123');
    expect(screen.getByText('METAR', { exact: true })).toBeInTheDocument();
    expect(screen.queryByText('REQ ATIS')).not.toBeInTheDocument();
    expect(screen.getByText('12 KT')).toBeInTheDocument();
    expect(screen.getByText('8 °C')).toBeInTheDocument();
    expect(screen.getByText('1012 hPa')).toBeInTheDocument();

    fireEvent.click(screen.getByText('METAR', { exact: true }));
    const dialog = await screen.findByRole('dialog', { name: 'ATIS' });
    expect(dialog).toHaveTextContent('ATIS offline - METAR for SAS123');
    expect(dialog).toHaveTextContent(metar);
  });

  it('shows NIL on the arrival tile when no guidance data is available', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(
      String(input).includes('/api/efb/me')
        ? { live_mode: true, callsign_locked: true, online_callsign: 'SAS123', training_available: false }
        : { callsign: 'SAS123', origin: 'EDDF', destination: 'EKCH', phase: 'ARRIVAL',
          capabilities: { pdc: false, tobt_update: false, stand_reassignment: false } }
    ), { status: 200 }))));
    render(<EFBPage />);
    await screen.findByText('SAS123');
    for (const name of ['Published holding', 'After passing', 'Expected APP', 'Expected STAR']) {
      expect(screen.getByRole('group', { name })).toBeInTheDocument();
    }
    expect(screen.getByRole('group', { name: 'Published holding' })).toHaveTextContent(/^PUBLISHED HOLDINGNIL$/);
    expect(screen.getByRole('group', { name: 'Expected APP' })).toHaveTextContent(/^EXPECTED APPNIL$/);
    expect(screen.getByRole('group', { name: 'Expected STAR' })).toHaveTextContent(/^EXPECTED STARNIL$/);
    expect(screen.getByAltText('Arrival procedure').closest('div')!.parentElement).toHaveTextContent(/^NIL$/);
  });
});
