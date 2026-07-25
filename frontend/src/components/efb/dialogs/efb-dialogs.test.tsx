import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import D1ArrivalBrief from './D1ArrivalBrief';
import D1Brief from './D1Brief';
import D1Chart from './D1Chart';
import D1Stand from './D1Stand';
import D2ATISDialog from './D2ATISDialog';
import D2CDMDialog from './D2CDMDialog';
import D2PDCDialog from './D2PDCDialog';

describe('EFB operational dialogs', () => {
  it('loads and displays the ChartFox chart matching the assigned SID', async () => {
    window.__APP_CONFIG__ = { chartfoxClientId: 'chartfox-client' };
    window.sessionStorage.setItem('chartfox.access-token', 'chartfox-token');
    window.sessionStorage.setItem('chartfox.access-token-expiry', String(Date.now() + 60_000));
    const sidPages = Array.from({ length: 5 }, (_, index) => ({
      id: index === 0 ? 'sid-chart' : `sid-chart-page-${index + 1}`,
      parent_id: index === 0 ? null : 'sid-chart',
      name: `RNP RWY 22 R - ${index + 1}`,
      type_key: 'SID',
      meta: [{ type_key: 'Runways', value: ['22'] }],
    }));
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/charts/grouped')) {
        return Promise.resolve(new Response(JSON.stringify({ data: {
          4: sidPages,
          5: { 1: { id: 'star-chart', name: 'NEXEN 2A', type_key: 'STAR' } },
        } }), { status: 200 }));
      }
      const page = sidPages.findIndex((chart) => url.endsWith(`/charts/${chart.id}`)) + 1;
      return Promise.resolve(new Response(JSON.stringify({ id: sidPages[page - 1].id, name: `RNP RWY 22 R - ${page}`, type_key: 'SID', url: `https://charts.example.test/odon1c-${page}.pdf`, allows_iframe: true }), { status: 200 }));
    });
    vi.stubGlobal('fetch', fetchMock);

    const onClose = vi.fn();
    const { rerender } = render(<D1Chart isOpen onClose={onClose} airport="EKCH" runway="22R" procedure="ODON1C" arrival={false} />);

    expect(await screen.findByTitle('RNP RWY 22 R - 1')).toHaveAttribute('src', 'https://charts.example.test/odon1c-1.pdf');
    expect(screen.getByLabelText('Chart page 1 of 5')).toBeInTheDocument();
    expect(fetchMock.mock.calls).toHaveLength(2);
    fireEvent.click(screen.getByRole('button', { name: 'Next chart' }));
    expect(await screen.findByTitle('RNP RWY 22 R - 2')).toHaveAttribute('src', 'https://charts.example.test/odon1c-2.pdf');
    expect(fetchMock.mock.calls[0][0]).toContain('/airports/EKCH/charts/grouped');
    expect(fetchMock.mock.calls[0][1].headers).toMatchObject({ Authorization: 'Bearer chartfox-token' });
    expect(fetchMock.mock.calls[1][0]).toContain('/charts/sid-chart');
    expect(fetchMock.mock.calls).toHaveLength(3);

    fireEvent.click(screen.getByRole('button', { name: 'CLICK TO CLOSE' }));
    rerender(<D1Chart isOpen={false} onClose={onClose} airport="EKCH" runway="22R" procedure="ODON1C" arrival={false} />);
    rerender(<D1Chart isOpen onClose={onClose} airport="EKCH" runway="22R" procedure="ODON1C" arrival={false} />);
    expect(await screen.findByTitle('RNP RWY 22 R - 1')).toBeInTheDocument();
    expect(fetchMock.mock.calls).toHaveLength(3);
  });

  it('shows only taxi, assigned arrival, and runway-matched approach charts', async () => {
    const charts = [
      { id: 'taxi-chart', name: 'Ground movement chart', type_key: 'GND' },
      { id: 'star-chart', name: 'TUDLO 4C', type_key: 'STAR', meta: [{ type_key: 'ProcedureIdent', value: ['TUDLO4C'] }] },
      { id: 'ils-22l', name: 'ILS Z RWY 22L', type_key: 'APP', meta: [{ type_key: 'Runways', value: ['22L'] }] },
      { id: 'ils-22l-page-2', parent_id: 'ils-22l', name: 'ILS Z RWY 22L - 2', type_key: 'APP', meta: [{ type_key: 'Runways', value: ['22L'] }] },
      { id: 'ils-22r', name: 'ILS Z RWY 22R', type_key: 'APP', meta: [{ type_key: 'Runways', value: ['22R'] }] },
      { id: 'unrelated-sid', name: 'NEXEN 2A', type_key: 'SID' },
    ];
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/charts/grouped')) return Promise.resolve(new Response(JSON.stringify({ data: { charts } }), { status: 200 }));
      const chart = charts.find((candidate) => url.endsWith(`/charts/${candidate.id}`));
      return Promise.resolve(new Response(JSON.stringify({ ...chart, url: `https://charts.example.test/${chart?.id}.pdf`, allows_iframe: true }), { status: 200 }));
    }));

    render(<D1Chart isOpen onClose={vi.fn()} airport="EKXX" runway="22L" procedure="TUDLO4C" arrival />);

    expect(await screen.findByTitle('TUDLO 4C')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'APPROACH 1' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'APPROACH 1' }));
    expect(await screen.findByTitle('ILS Z RWY 22L')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'ILS Z RWY 22L' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'ILS Z RWY 22R' })).not.toBeInTheDocument();
    expect(screen.getByLabelText('Chart page 1 of 2')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Next chart' }));
    expect(await screen.findByTitle('ILS Z RWY 22L - 2')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'AIRPORT / TAXI 1' }));
    expect(await screen.findByTitle('Ground movement chart')).toBeInTheDocument();
  });

  it('uses the selected stand, runway, and SID briefing assets', () => {
    render(<D1Brief isOpen onClose={vi.fn()} stand="A12" sid="NEXEN2A" runway="22R" />);

    expect(screen.getByRole('dialog', { name: 'Departure briefing' })).toBeInTheDocument();
    expect(screen.getByAltText('Pushback readiness').getAttribute('src')).toContain('.webp');

    fireEvent.click(screen.getByRole('button', { name: 'Go to Pushback from A12' }));
    expect(screen.getByAltText('Pushback guidance for stand A12').getAttribute('src')).toContain('A12-A17');
    expect(screen.getByText(/A12: expect Y1/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Go to Initial taxi: 22R' }));
    expect(screen.getByAltText('Initial taxi guidance for runway 22R').getAttribute('src')).toContain('TAXIINIT22R');

    fireEvent.click(screen.getByRole('button', { name: 'Go to SID: NEXEN2A' }));
    expect(screen.getByAltText('22R SID chart for NEXEN2A').getAttribute('src')).toContain('NEX-KOP-LAN-22');
    expect(screen.getByText(/Kastrup Departure on 124.980/)).toBeInTheDocument();
  });

  it('uses the selected stand, runway, and STAR arrival briefing assets', () => {
    render(<D1ArrivalBrief isOpen onClose={vi.fn()} stand="A12" star="TUDLO4C" runway="22L" holdingFix="LUGAS" holdingDetail="073/LEFT" terminalFix="ABEGI" arrivalHeading="HDG037" />);

    expect(screen.getByRole('dialog', { name: 'Arrival briefing' })).toBeInTheDocument();
    expect(screen.getByAltText('Arrival briefing introduction').getAttribute('src')).toContain('.webp');

    fireEvent.click(screen.getByRole('button', { name: 'Go to STAR and holding: TUDLO4C' }));
    expect(screen.getByAltText('TUDLO4C holding guidance for runway 22L').getAttribute('src')).toContain('hold-tudlo-22');
    expect(screen.getByText(/The holding for TUDLO4C is LUGAS/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Go to STAR restrictions' }));
    expect(screen.getByAltText('TUDLO4C restriction guidance for runway 22L').getAttribute('src')).toContain('arr-tudlo-22');
    expect(screen.getByText(/never turn inbound after ABEGI/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Go to Taxi to stand A12' }));
    expect(screen.getByAltText('Taxi-in guidance for Bravo stands on runway 22L').getAttribute('src')).toContain('taxiin-22bravo');
  });

  it('shows current stand availability and keeps a rejected request open', async () => {
    const onClose = vi.fn();
    const onRequest = vi.fn().mockRejectedValue(new Error('stand is occupied'));
    render(<D1Stand isOpen onClose={onClose} stand="A12" onRequest={onRequest} availability={[
      { stand: 'A18', available: true },
      { stand: 'A19', available: false, reason: 'reserved by SAS999' },
    ]} />);

    expect(screen.getByTitle('A18: Available for this flight')).toBeInTheDocument();
    expect(screen.getByTitle('A19: Unavailable: reserved by SAS999')).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'Stand A18' }));
    fireEvent.click(screen.getByRole('button', { name: 'REQUEST NEW STAND' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('stand is occupied');
    expect(onClose).not.toHaveBeenCalled();
  });

  it('does not submit an initial stand missing from the availability response', () => {
    const onRequest = vi.fn();
    render(<D1Stand isOpen onClose={vi.fn()} stand="NIL" onRequest={onRequest} availability={[
      { stand: 'A18', available: true },
    ]} />);

    const request = screen.getByRole('button', { name: 'REQUEST NEW STAND' });
    expect(request).toBeDisabled();
    fireEvent.click(request);
    expect(onRequest).not.toHaveBeenCalled();
  });

  it('shows ATIS as read-only current information', () => {
    render(<D2ATISDialog isOpen onClose={vi.fn()} position="L3" content="ATIS for SAS123" atisText="INFORMATION D" />);

    expect(screen.getByText('INFORMATION D')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'CLOSE' })).toBeInTheDocument();
    expect(screen.queryByText('ACKNOWLEDGE')).not.toBeInTheDocument();
  });

  it('keeps PDC open and displays a failed confirmation', async () => {
    const onClose = vi.fn();
    render(
      <D2PDCDialog
        isOpen
        onClose={onClose}
        onConfirm={vi.fn().mockRejectedValue(new Error('clearance changed'))}
        onUnable={vi.fn().mockResolvedValue(undefined)}
        position="L3"
        content="PDC for SAS123"
        pdcText="CLEARED AS FILED"
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'CONFIRM' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('clearance changed');
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'UNABLE' })).toBeInTheDocument();
  });

  it('rejects invalid TOBT locally and reports server rejection without closing', async () => {
    const onClose = vi.fn();
    const onUpdate = vi.fn().mockRejectedValue(new Error('CDM unavailable'));
    render(<D2CDMDialog isOpen onClose={onClose} currentTobt="1425Z" currentCtot="NIL" onUpdate={onUpdate} />);

    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: '9999Z' } });
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    expect(screen.getByRole('alert')).toHaveTextContent('valid UTC time');
    expect(onUpdate).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { value: '1430Z' } });
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith('1430Z'));
    expect(await screen.findByRole('alert')).toHaveTextContent('CDM unavailable');
    expect(onClose).not.toHaveBeenCalled();
  });
});
