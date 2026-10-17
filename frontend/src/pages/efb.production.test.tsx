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
    expect(screen.getByRole('group', { name: 'Expected STAR' })).toHaveTextContent('ROUTE / ATIS');
    expect(screen.getByText('Maintain HDG037')).toBeInTheDocument();
    await waitFor(() => expect(window.sessionStorage.getItem('efb-training-session')).toBeNull());
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('training_session=') || String(url).includes('callsign='))).toBe(false);
    fireEvent.click(screen.getByRole('button', { name: 'Open arrival briefing' }));
    expect(screen.getByText(/NOT AN ATC CLEARANCE/)).toBeInTheDocument();
  });
});
