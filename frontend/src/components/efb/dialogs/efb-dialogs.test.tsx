import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import D1ArrivalBrief from './D1ArrivalBrief';
import D1Brief from './D1Brief';
import D1Chart from './D1Chart';
import D1Stand from './D1Stand';
import D2ATISDialog from './D2ATISDialog';
import D2CDMDialog from './D2CDMDialog';
import D2PDCDialog from './D2PDCDialog';

describe('EFB operational dialogs', () => {
  it.each([
    ['UNKNOWN', '22L'],
    ['TUDLO', 'NIL'],
    ['TUDLO', '12'],
    ['TUDLO', '22R'],
    ['TUDLO', '04R'],
  ])('does not invent briefing guidance for STAR %s and runway %s', (star, runway) => {
    render(<D1ArrivalBrief isOpen onClose={vi.fn()} stand="TBA" star={star} runway={runway} holdingFix="" holdingDetail="" terminalFix="" arrivalHeading="" />);
    expect(screen.getByRole('heading', { name: 'Arrival guidance unavailable' })).toBeInTheDocument();
    expect(screen.getByText(/No procedure has been assumed/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Go to STAR and holding/ })).not.toBeInTheDocument();
  });

  it.each(['NIL', '12', '30', '04L', '22L', '04R/22R'])('does not substitute departure runway %s with 04R guidance', (runway) => {
    render(<D1Brief isOpen onClose={vi.fn()} stand="A12" sid="NEXEN2A" runway={runway} />);
    expect(screen.getByRole('heading', { name: 'Departure guidance unavailable' })).toBeInTheDocument();
    expect(screen.getByText(/No runway has been assumed/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Go to Initial taxi/ })).not.toBeInTheDocument();
  });

  it('labels fallback guidance as expected and does not invent taxi routing to an unassigned stand', () => {
    render(<D1ArrivalBrief isOpen onClose={vi.fn()} expected stand="TBA" star="TUDLO" runway="22L" holdingFix="LUGAS" holdingDetail="077/LEFT" terminalFix="ABEGI" arrivalHeading="HDG037" />);
    expect(screen.getByText(/EXPECTED FROM ROUTE \/ ATIS - NOT AN ATC CLEARANCE/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Go to STAR and holding: TUDLO' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Go to Taxi to stand/ })).not.toBeInTheDocument();
  });

  it('keeps expected arrival charts distinct from ATC-assigned procedures', () => {
    render(<D1Chart isOpen onClose={vi.fn()} airport="EKCH" runway="22L" procedure="TUDLO" arrival expected />);
    expect(screen.getByRole('button', { name: 'EXPECTED STAR 0' })).toBeInTheDocument();
    expect(screen.getByText(/not an ATC clearance. Confirm the procedure and version with ATC/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /ASSIGNED STAR/ })).not.toBeInTheDocument();
  });

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

  it.each([false, true])('supports briefing keyboard navigation, page transitions, wraparound, and closing (arrival: %s)', (arrival) => {
    const onClose = vi.fn();
    render(arrival
      ? <D1ArrivalBrief isOpen onClose={onClose} stand="A12" star="TUDLO4C" runway="22L" holdingFix="LUGAS" holdingDetail="073/LEFT" terminalFix="ABEGI" arrivalHeading="HDG037" />
      : <D1Brief isOpen onClose={onClose} stand="A12" sid="NEXEN2A" runway="22R" />);

    const dialog = screen.getByRole('dialog');
    const nav = screen.getByRole('navigation', { name: 'Briefing navigation' });
    const steps = within(nav).getAllByRole('button').filter((button) => button.getAttribute('aria-label')?.startsWith('Go to '));
    const firstImage = within(dialog).getByRole('img', { hidden: false, name: arrival ? 'Arrival briefing introduction' : 'Pushback readiness' });
    const firstPage = firstImage.closest('.efb-briefing-page');
    expect(firstImage).toHaveClass('rounded-xl');
    expect(firstPage).not.toBeNull();
    expect(steps[0]).toHaveAttribute('aria-current', 'step');
    expect(nav).toHaveClass('justify-center', 'gap-3');
    for (const step of steps) {
      expect(step.textContent).toBe('');
      expect(step.firstElementChild).toHaveClass('rounded-full', 'size-4');
    }
    const paragraphs = dialog.querySelectorAll('.efb-briefing-page p');
    expect(screen.getAllByRole('separator')).toHaveLength(paragraphs.length - 1);

    fireEvent.click(screen.getByRole('button', { name: 'Next briefing page' }));
    expect(steps[1]).toHaveAttribute('aria-current', 'step');
    expect(firstPage).not.toBeInTheDocument();
    expect(dialog.querySelector('.efb-briefing-page')).toBeInTheDocument();
    expect(dialog.querySelector('.efb-briefing-page')).toHaveAttribute('data-direction', 'forward');
    fireEvent.keyDown(dialog, { key: 'ArrowLeft' });
    expect(steps[0]).toHaveAttribute('aria-current', 'step');
    expect(dialog.querySelector('.efb-briefing-page')).toHaveAttribute('data-direction', 'backward');
    fireEvent.keyDown(dialog, { key: 'ArrowLeft' });
    expect(steps.at(-1)).toHaveAttribute('aria-current', 'step');
    expect(dialog.querySelector('.efb-briefing-page')).toHaveAttribute('data-direction', 'backward');
    expect(screen.getByRole('button', { name: 'Next briefing page' })).toHaveTextContent('START AGAIN');
    fireEvent.keyDown(dialog, { key: 'ArrowRight' });
    expect(steps[0]).toHaveAttribute('aria-current', 'step');
    expect(dialog.querySelector('.efb-briefing-page')).toHaveAttribute('data-direction', 'forward');
    fireEvent.click(steps[2]);
    expect(dialog.querySelector('.efb-briefing-page')).toHaveAttribute('data-direction', 'forward');
    fireEvent.click(steps[1]);
    expect(dialog.querySelector('.efb-briefing-page')).toHaveAttribute('data-direction', 'backward');

    fireEvent.click(screen.getByRole('button', { name: 'CLICK TO CLOSE' }));
    expect(onClose).toHaveBeenCalledTimes(1);
    fireEvent.keyDown(dialog, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it('shows current stand availability and keeps a rejected request open', async () => {
    const onClose = vi.fn();
    const onRequest = vi.fn().mockRejectedValue(new Error('stand is occupied'));
    render(<D1Stand isOpen onClose={onClose} stand="A12" onRequest={onRequest} availability={[
      { stand: 'A18', available: true },
      { stand: 'A19', available: false, reason: 'reserved by SAS999', reserved_by: ['SAS999'] },
    ]} />);

    expect(screen.getByTitle('A18: Available for this flight')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Stand A18' })).toHaveClass('bg-[#41826E]');
    expect(screen.getByRole('button', { name: 'Stand A19' })).toHaveTextContent('SAS999');
    expect(screen.getByRole('button', { name: 'Stand A19' })).toHaveClass('bg-[#622E3A]');
    expect(screen.getByTitle('A19: Unavailable: reserved by SAS999')).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'Stand A18' }));
    fireEvent.click(screen.getByRole('button', { name: 'REQUEST NEW STAND' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('stand is occupied');
    expect(onClose).not.toHaveBeenCalled();
  });

  it('preserves the stand map and distinguishes reservations from adjacent blocks', () => {
    render(<D1Stand isOpen onClose={vi.fn()} stand="A18" onRequest={vi.fn()} availability={[
      { stand: 'A18', available: true },
      { stand: 'A19', available: false, reason: 'reserved by SAS999; reserved by SAS888', reserved_by: ['SAS888', 'SAS999'] },
      { stand: 'A20', available: false, reason: 'blocked by allocated neighbor A19' },
      { stand: 'E82', available: true },
    ]} />);
    expect(screen.getByRole('dialog', { name: 'SELECT STAND' })).toHaveClass('w-[98vw]', 'h-[96dvh]');
    expect(screen.getByRole('dialog', { name: 'SELECT STAND' })).not.toHaveClass('max-w-6xl');
    expect(screen.getByText('Choose a stand on the map. Gray stands require suitability confirmation; reserved or blocked stands cannot be selected.')).toHaveClass('sr-only');
    expect(screen.getByRole('dialog', { name: 'SELECT STAND' }).querySelector('header')).toHaveClass('py-[12.8px]', 'px-4', 'sm:px-[22.4px]');
    expect(screen.getByRole('group', { name: 'Airport stand map' })).toHaveClass('w-full', 'flex-1', 'min-h-0');
    expect(screen.getByRole('group', { name: 'Airport stand map' })).not.toHaveClass('aspect-[16/9]', 'min-w-[900px]');
    expect(screen.getByRole('group', { name: 'Airport stand map' }).parentElement).not.toHaveClass('overflow-auto');
    expect(screen.getByRole('button', { name: 'Stand E82' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Stand A18' }).style.getPropertyValue('--stand-left')).toBe(`${461.851 / 2560 * 100}%`);
    expect(screen.getByRole('button', { name: 'Stand A18' }).style.getPropertyValue('--stand-top')).toBe(`${229 / 1440 * 100}%`);
    expect(screen.getByText('SAS888')).toHaveClass('block', 'whitespace-nowrap');
    expect(screen.getByText('SAS999')).toHaveClass('block', 'whitespace-nowrap');
    expect(screen.getByRole('button', { name: 'Stand A20' })).toHaveAttribute('title', 'A20: Unavailable: blocked by allocated neighbor A19');
    expect(screen.getByRole('button', { name: 'Stand A20' })).not.toHaveTextContent('RESERVED');
    expect(screen.getByRole('status', { name: 'Stand availability summary' })).toHaveTextContent('2 AVAILABLE');
    expect(screen.getByRole('button', { name: 'Stand A18' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getAllByRole('img', { name: 'Selected stand' })).toHaveLength(1);
    expect(screen.getByRole('button', { name: 'Stand A18' })).toContainElement(screen.getByRole('img', { name: 'Selected stand' }));
    expect(screen.getByRole('group', { name: 'Airport stand map' }).querySelectorAll('.rounded-full')).toHaveLength(0);
    fireEvent.click(screen.getByRole('button', { name: 'Stand E82' }));
    expect(screen.getAllByRole('img', { name: 'Selected stand' })).toHaveLength(1);
    expect(screen.getByRole('button', { name: 'Stand E82' })).toContainElement(screen.getByRole('img', { name: 'Selected stand' }));
  });

  it('requires confirmation for unsuitable stands and leaves blocks protected', async () => {
    const onRequest = vi.fn().mockRejectedValue(new Error('stand has just been reserved'));
    render(<D1Stand isOpen onClose={vi.fn()} stand="TBA" onRequest={onRequest} availability={[
      { stand: 'A18', available: true, selectable: true },
      { stand: 'A19', available: false, unsuitable: true, selectable: true, reason: 'incompatible WTC' },
      { stand: 'A20', available: false, unsuitable: true, selectable: false, reason: 'manually blocked' },
      { stand: 'A21', available: false, unsuitable: true, selectable: false, reserved_by: ['SAS999'] },
    ]} />);
    expect(screen.getByText('NOT SUITABLE')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Stand A20' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Stand A21' })).toBeDisabled();
    const unsuitable = screen.getByRole('button', { name: 'Stand A19' });
    expect(unsuitable).toBeEnabled();
    expect(unsuitable).toHaveClass('bg-[#374151]');
    fireEvent.click(unsuitable);
    fireEvent.click(screen.getByRole('button', { name: 'REQUEST NEW STAND' }));
    let warning = screen.getByRole('dialog', { name: 'Stand not suitable' });
    expect(warning).toHaveTextContent('This stand is not suitable for your flight. Select anyway?');
    expect(onRequest).not.toHaveBeenCalled();
    fireEvent.click(within(warning).getByRole('button', { name: 'CANCEL' }));
    expect(screen.queryByRole('dialog', { name: 'Stand not suitable' })).not.toBeInTheDocument();
    expect(onRequest).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'REQUEST NEW STAND' })).toBeDisabled();
    fireEvent.click(unsuitable);
    fireEvent.click(screen.getByRole('button', { name: 'REQUEST NEW STAND' }));
    warning = screen.getByRole('dialog', { name: 'Stand not suitable' });
    fireEvent.click(within(warning).getByRole('button', { name: 'SELECT ANYWAY' }));
    await waitFor(() => expect(onRequest).toHaveBeenCalledWith('A19', true));
    expect(await screen.findByRole('alert')).toHaveTextContent('stand has just been reserved');
    fireEvent.click(screen.getByRole('button', { name: 'Stand A18' }));
    fireEvent.click(screen.getByRole('button', { name: 'REQUEST NEW STAND' }));
    await waitFor(() => expect(onRequest).toHaveBeenLastCalledWith('A18'));
    expect(screen.queryByRole('dialog', { name: 'Stand not suitable' })).not.toBeInTheDocument();
  });

  it.each(['cancel', 'escape'])('restores the previous selection and its map after warning %s', (dismissal) => {
    const onRequest = vi.fn();
    const onClose = vi.fn();
    render(<D1Stand isOpen onClose={onClose} stand="A18" onRequest={onRequest} availability={[
      { stand: 'A18', available: true },
      { stand: 'A19', available: true },
      { stand: 'G117', available: false, unsuitable: true, selectable: true },
      { stand: 'G118', available: false, unsuitable: true, selectable: true },
    ]} />);
    fireEvent.click(screen.getByRole('button', { name: 'Stand A19' }));
    fireEvent.click(screen.getByRole('button', { name: 'CARGO' }));
    fireEvent.click(screen.getByRole('button', { name: 'Stand G117' }));
    fireEvent.click(screen.getByRole('button', { name: 'Stand G118' }));
    fireEvent.click(screen.getByRole('button', { name: 'REQUEST NEW STAND' }));
    const warning = screen.getByRole('dialog', { name: 'Stand not suitable' });
    if (dismissal === 'cancel') fireEvent.click(within(warning).getByRole('button', { name: 'CANCEL' }));
    else fireEvent.keyDown(warning, { key: 'Escape' });
    expect(screen.queryByRole('dialog', { name: 'Stand not suitable' })).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Airport stand map' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Stand A19' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText(/^SELECTED$/)).toHaveTextContent('A19');
    expect(screen.getAllByRole('img', { name: 'Selected stand' })).toHaveLength(1);
    expect(onRequest).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it('omits removed stands and uses E36 at the former E26 position', () => {
    render(<D1Stand isOpen onClose={vi.fn()} stand="TBA" onRequest={vi.fn()} availability={[
      ...['B3', 'B5', 'B16', 'A50', 'C26', 'E26', 'E36'].map((stand) => ({ stand, available: true })),
    ]} />);
    for (const stand of ['B3', 'B5', 'B16', 'A50', 'C26', 'E26']) {
      expect(screen.queryByRole('button', { name: `Stand ${stand}` })).not.toBeInTheDocument();
    }
    const e36 = screen.getByRole('button', { name: 'Stand E36' });
    expect(e36).toBeEnabled();
    expect(e36.style.getPropertyValue('--stand-left')).toBe(`${1844 / 2560 * 100}%`);
    expect(e36.style.getPropertyValue('--stand-top')).toBe(`${626.939 / 1440 * 100}%`);
    expect(screen.getByRole('status', { name: 'Stand availability summary' })).toHaveTextContent('1 AVAILABLE');
    fireEvent.click(e36);
    expect(e36).toHaveAttribute('aria-pressed', 'true');
  });

  it('switches between apron and cargo maps while retaining the selected stand', async () => {
    const onRequest = vi.fn().mockResolvedValue(undefined);
    render(<D1Stand isOpen onClose={vi.fn()} stand="A18" onRequest={onRequest} availability={[
      { stand: 'A18', available: true },
      { stand: 'G117', available: true },
      { stand: 'G118', available: false, reserved_by: ['DHL123'], reason: 'reserved by DHL123' },
      { stand: 'G19', available: false, unsuitable: true, selectable: true, reason: 'incompatible WINGSPAN' },
    ]} />);
    expect(screen.queryByText(/Request stand .* for your aircraft/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'CARGO' })).toHaveClass('min-w-32', 'border-[#B8D9F0]/35', 'hover:bg-white/10');
    fireEvent.click(screen.getByRole('button', { name: 'CARGO' }));
    const cargo = screen.getByRole('group', { name: 'Cargo stand map' });
    expect(screen.getByRole('button', { name: 'APRON' })).toHaveClass('min-w-32', 'border-[#B8D9F0]/35', 'hover:bg-white/10');
    expect(within(cargo).getAllByRole('button')).toHaveLength(31);
    expect(screen.queryByRole('group', { name: 'Airport stand map' })).not.toBeInTheDocument();
    const g117 = screen.getByRole('button', { name: 'Stand G117' });
    expect(g117.style.getPropertyValue('--stand-left')).toBe(`${185 / 1566 * 100}%`);
    expect(g117.style.getPropertyValue('--stand-top')).toBe(`${12 / 707 * 100}%`);
    expect(screen.getByRole('button', { name: 'Stand G118' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Stand G118' })).toHaveTextContent('DHL123');
    expect(screen.getByRole('button', { name: 'Stand G19' })).toBeEnabled();
    expect(screen.getByRole('status', { name: 'Stand availability summary' })).toHaveTextContent('1 AVAILABLE');
    fireEvent.click(g117);
    fireEvent.click(screen.getByRole('button', { name: 'APRON' }));
    expect(screen.getByRole('group', { name: 'Airport stand map' })).toBeInTheDocument();
    expect(screen.getByText(/^SELECTED$/)).toHaveTextContent('G117');
    fireEvent.click(screen.getByRole('button', { name: 'CARGO' }));
    expect(screen.getByRole('button', { name: 'Stand G117' })).toHaveAttribute('aria-pressed', 'true');
    fireEvent.click(screen.getByRole('button', { name: 'Stand G19' }));
    fireEvent.click(screen.getByRole('button', { name: 'REQUEST NEW STAND' }));
    fireEvent.click(screen.getByRole('button', { name: 'SELECT ANYWAY' }));
    await waitFor(() => expect(onRequest).toHaveBeenCalledWith('G19', true));
  });

  it('opens an existing cargo assignment directly on the cargo map', () => {
    render(<D1Stand isOpen onClose={vi.fn()} stand="G110" availability={[{ stand: 'G110', available: true }]} />);
    expect(screen.getByRole('group', { name: 'Cargo stand map' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'APRON' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Stand G110' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('keeps unavailable and loading stands non-interactive', () => {
    const onRequest = vi.fn();
    const view = render(<D1Stand isOpen onClose={vi.fn()} stand="A18" onRequest={onRequest} availabilityLoading />);
    expect(screen.getByRole('button', { name: 'LOADING AVAILABILITY' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Stand A18' })).toBeDisabled();
    view.rerender(<D1Stand isOpen onClose={vi.fn()} stand="A18" onRequest={onRequest} availability={[{ stand: 'A18', available: true }]} availabilityUnavailable />);
    expect(screen.getByRole('alert')).toHaveTextContent('Stand availability is currently unavailable');
    expect(screen.getByRole('button', { name: 'REQUEST NEW STAND' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Stand A18' })).toBeDisabled();
    expect(onRequest).not.toHaveBeenCalled();
  });

  it('confirms a selected stand and closes with Escape or cancel', async () => {
    const onClose = vi.fn();
    const onRequest = vi.fn().mockResolvedValue(undefined);
    render(<D1Stand isOpen onClose={onClose} stand="TBA" onRequest={onRequest} availability={[{ stand: 'A18', available: true }]} />);
    fireEvent.click(screen.getByRole('button', { name: 'Stand A18' }));
    expect(screen.getByRole('button', { name: 'Stand A18' })).toHaveAttribute('aria-pressed', 'true');
    fireEvent.click(screen.getByRole('button', { name: 'REQUEST NEW STAND' }));
    await waitFor(() => expect(onRequest).toHaveBeenCalledWith('A18'));
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(2);
    fireEvent.click(screen.getByRole('button', { name: 'CANCEL' }));
    expect(onClose).toHaveBeenCalledTimes(3);
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

  it('starts at current UTC, steps across midnight, and resets when reopened', () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(new Date('2026-06-15T23:58:00Z'));
      const props = { onClose: vi.fn(), currentTobt: '1425Z', currentCtot: 'NIL', onUpdate: vi.fn() };
      const { rerender } = render(<D2CDMDialog isOpen {...props} />);
      expect(screen.getByLabelText('New TOBT (UTC)')).toHaveValue('2358Z');
      fireEvent.click(screen.getByRole('button', { name: 'Increase TOBT by 5 minutes' }));
      expect(screen.getByLabelText('New TOBT (UTC)')).toHaveValue('0003Z');
      fireEvent.click(screen.getByRole('button', { name: 'Decrease TOBT by 5 minutes' }));
      expect(screen.getByLabelText('New TOBT (UTC)')).toHaveValue('2358Z');
      rerender(<D2CDMDialog isOpen={false} {...props} />);
      vi.setSystemTime(new Date('2026-06-16T00:10:00Z'));
      rerender(<D2CDMDialog isOpen {...props} />);
      expect(screen.getByLabelText('New TOBT (UTC)')).toHaveValue('0010Z');
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not offer a real update without a submission handler', () => {
    render(<D2CDMDialog isOpen onClose={vi.fn()} currentTobt="1425Z" currentCtot="NIL" />);
    expect(screen.getByRole('button', { name: 'UPDATE TOBT' })).toBeDisabled();
    expect(screen.getByText(/no update will be sent to vIFF/)).toBeInTheDocument();
  });

  it('waits for a confirmed TOBT update before closing', async () => {
    const onClose = vi.fn();
    let confirm!: () => void;
    const onUpdate = vi.fn(() => new Promise<void>((resolve) => { confirm = resolve; }));
    render(<D2CDMDialog isOpen onClose={onClose} currentTobt="1425Z" currentCtot="NIL" onUpdate={onUpdate} />);
    fireEvent.change(screen.getByLabelText('New TOBT (UTC)'), { target: { value: '1430Z' } });
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    expect(onUpdate).toHaveBeenCalledWith('1430Z');
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'UPDATING TOBT' })).toBeDisabled();
    confirm();
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
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
