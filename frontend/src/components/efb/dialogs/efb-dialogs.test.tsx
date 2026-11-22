import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import D1ArrivalBrief from './D1ArrivalBrief';
import D1Brief from './D1Brief';
import D1Chart from './D1Chart';
import D1OptionsDialog from './D1OptionsDialog';
import D1Stand from './D1Stand';
import D2ATISDialog from './D2ATISDialog';
import D2CDMDialog from './D2CDMDialog';
import D2PDCDialog from './D2PDCDialog';
import { computeTaxiRoute } from '../taxi/taxiRoute';

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
    expect(screen.getByText(/EXPECTED FROM ROUTE \/ ATIS \/ WIND - NOT AN ATC CLEARANCE/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Go to STAR and holding: TUDLO' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Go to Taxi to stand/ })).not.toBeInTheDocument();
  });

  it('keeps expected arrival charts distinct from ATC-assigned procedures', () => {
    render(<D1Chart isOpen onClose={vi.fn()} airport="EKCH" runway="22L" procedure="TUDLO" arrival expected />);
    expect(screen.getByRole('button', { name: 'EXPECTED STAR 0' })).toBeInTheDocument();
    expect(screen.getByText(/not an ATC clearance. Confirm the procedure and version with ATC/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /ASSIGNED STAR/ })).not.toBeInTheDocument();
  });

  it('offers the taxi clearance only for departures with a taxi network', () => {
    const { rerender } = render(<D1Chart isOpen onClose={vi.fn()} airport="EKCH" runway="22R" procedure="ODON1C" arrival={false} />);
    expect(screen.queryByRole('button', { name: 'TAXI CLEARANCE' })).not.toBeInTheDocument();
    rerender(<D1Chart isOpen onClose={vi.fn()} airport="EKCH" runway="22R" procedure="ODON1C" arrival={false} taxiClearance={{ callsign: 'SAS123', releasePoint: null }} />);
    expect(screen.getByRole('button', { name: 'TAXI CLEARANCE' })).toBeInTheDocument();
  });

  it('starts the taxi route from the ATC release point and draws the typed clearance', () => {
    const props = { isOpen: true, onClose: vi.fn(), airport: 'EKCH', runway: '22R', procedure: 'ODON1C', arrival: false } as const;
    const { rerender } = render(<D1Chart {...props} taxiClearance={{ callsign: 'SAS123', releasePoint: 'z3' }} />);
    fireEvent.click(screen.getByRole('button', { name: 'TAXI CLEARANCE' }));

    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('Z3');
    expect(screen.getByText('ATC', { selector: 'span' })).toBeInTheDocument();
    expect(screen.getByText('No runway assigned')).toBeInTheDocument();
    // The help table explains the syntax without offering "," (still accepted as hold short).
    const help = screen.getByRole('table');
    expect(within(help).getByText('Hold short of')).toBeInTheDocument();
    expect(help).not.toHaveTextContent(',');
    expect(screen.getByRole('img', { name: 'Taxi route' })).toBeInTheDocument();

    fireEvent.change(screen.getByRole('textbox', { name: 'TAXI CLEARANCE' }), { target: { value: 'KZAx30A4.22R' } });
    expect(screen.getByLabelText('Clearance parts')).toHaveTextContent('Z3›K›Z›A›X 30›A4›HS 22R');
    expect(screen.queryByRole('button', { name: /FLIP/ })).not.toBeInTheDocument();
    expect(screen.getByText('Route OK.')).toBeInTheDocument();
    // No runway on the flight: the hold short of 22R in the clearance gives the departure runway.
    expect(screen.getByLabelText('DEP RWY')).toHaveValue('');
    expect(screen.getByText('CLEARANCE', { selector: 'span' })).toBeInTheDocument();
    expect(screen.getByText('No common routes for runway 22R yet.')).toBeInTheDocument();
    expect(within(screen.getByRole('img', { name: 'Taxi route' })).getByText('RWY 22R')).toBeInTheDocument();
    const map = within(screen.getByRole('img', { name: 'Taxi route' }));
    expect(map.getByText('HOLD SHORT 22R')).toBeInTheDocument();
    expect(map.getByText('CROSS 30')).toBeInTheDocument();

    // "/" starts an onward clearance: what came before turns grey.
    fireEvent.change(screen.getByRole('textbox', { name: 'TAXI CLEARANCE' }), { target: { value: 'KZ.A/Ax30A4.22R' } });
    expect(screen.getByLabelText('Clearance parts')).toHaveTextContent('Z3›K›Z›HS A/A›X 30›A4›HS 22R');
    expect(screen.getByRole('img', { name: 'Taxi route' }).querySelectorAll('polyline[stroke="#d3d8df"]').length).toBeGreaterThan(0);
    // Taxiway names are yellow signs that never overlap each other.
    const signs = Array.from(screen.getByRole('img', { name: 'Taxi route' }).querySelectorAll('rect[fill="#e9e814"], rect[fill="#d3d8df"]'), (rect) => {
      const [x, y, w, hgt] = ['x', 'y', 'width', 'height'].map((key) => Number(rect.getAttribute(key)));
      return { x0: x, y0: y, x1: x + w, y1: y + hgt };
    });
    expect(signs.length).toBe(4);
    for (const [i, p] of signs.entries()) for (const q of signs.slice(i + 1)) expect(p.x0 < q.x1 && q.x0 < p.x1 && p.y0 < q.y1 && q.y0 < p.y1).toBe(false);

    // The aircraft moves to where the active (magenta) part begins.
    const activeStart = computeTaxiRoute('Z3', 'KZ.A/Ax30A4.22R').legs.find((leg) => !leg.past)!.pts[0];
    const aircraft = screen.getByRole('img', { name: 'Taxi route' }).querySelector('g[transform*="rotate"]')!;
    expect(aircraft.getAttribute('transform')).toContain(`translate(${activeStart[0]} ${activeStart[1]})`);

    // A clearance that does not connect is reported, not thrown.
    fireEvent.change(screen.getByRole('textbox', { name: 'TAXI CLEARANCE' }), { target: { value: 'KBx30ZA4.22R' } });
    expect(screen.getByText('K does not connect to B')).toBeInTheDocument();
    fireEvent.change(screen.getByRole('textbox', { name: 'TAXI CLEARANCE' }), { target: { value: 'KZAx30A4.22R' } });

    // ATC moves the release point: followed while the pilot has not typed their own.
    rerender(<D1Chart {...props} taxiClearance={{ callsign: 'SAS123', releasePoint: 'Z4' }} />);
    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('Z4');

    // Once the pilot types their own, an ATC change is offered instead of overwriting it.
    fireEvent.change(screen.getByLabelText('RELEASE POINT'), { target: { value: 'z2' } });
    rerender(<D1Chart {...props} taxiClearance={{ callsign: 'SAS123', releasePoint: 'Z5' }} />);
    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('Z2');
    fireEvent.click(screen.getByRole('button', { name: 'USE Z5' }));
    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('Z5');

    rerender(<D1Chart {...props} taxiClearance={{ callsign: 'SAS123', releasePoint: null }} />);
    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('');
    expect(screen.queryByText('ATC', { selector: 'span' })).not.toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'TAXI CLEARANCE' })).toHaveValue('KZAx30A4.22R');

    // A new flight starts over.
    rerender(<D1Chart {...props} taxiClearance={{ callsign: 'SAS456', releasePoint: null }} />);
    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('');
    expect(screen.getByRole('textbox', { name: 'TAXI CLEARANCE' })).toHaveValue('');
  });

  it('uses the pilot runway override for common taxi routes and offers button zoom', () => {
    render(<D1Chart isOpen onClose={vi.fn()} airport="EKCH" runway="22R" procedure="ODON1C" arrival={false}
      taxiClearance={{ callsign: 'SAS123', releasePoint: 'Z3', runway: '22R' }} />);

    expect(screen.getByText('No common routes for runway 22R yet.')).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('DEP RWY'), { target: { value: '04R' } });
    expect(screen.getByText('No common routes for runway 04R yet.')).toBeInTheDocument();

    const map = screen.getByRole('img', { name: 'Taxi route' });
    const width = () => Number(map.getAttribute('viewBox')?.split(' ')[2]);
    const initialWidth = width();
    fireEvent.click(screen.getByRole('button', { name: 'Zoom in on taxi map' }));
    expect(width()).toBeLessThan(initialWidth);
    fireEvent.click(screen.getByRole('button', { name: 'Zoom out on taxi map' }));
    expect(width()).toBeCloseTo(initialWidth);

    fireEvent.change(screen.getByLabelText('DEP RWY'), { target: { value: 'NIL' } });
    expect(screen.getByText('No runway assigned')).toBeInTheDocument();
  });

  it('uses the stand default release point ahead of ATC for G stands without release points', () => {
    const props = { isOpen: true, onClose: vi.fn(), airport: 'EKCH', runway: '22L', procedure: 'ODON1C', arrival: false } as const;
    const { rerender } = render(<D1Chart {...props} taxiClearance={{ callsign: 'SAS123', releasePoint: 'T3', stand: 'G112' }} />);
    fireEvent.click(screen.getByRole('button', { name: 'TAXI CLEARANCE' }));
    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('I');
    expect(screen.getByText('STAND', { selector: 'span' })).toBeInTheDocument();

    rerender(<D1Chart {...props} taxiClearance={{ callsign: 'SAS123', releasePoint: 'T3', stand: 'G120' }} />);
    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('G6');

    fireEvent.change(screen.getByLabelText('RELEASE POINT'), { target: { value: 'G' } });
    rerender(<D1Chart {...props} taxiClearance={{ callsign: 'SAS123', releasePoint: 'T3', stand: 'G111' }} />);
    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('G');
    expect(screen.getByText('Default for stand G111')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'USE I' }));
    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('I');

    rerender(<D1Chart {...props} taxiClearance={{ callsign: 'SAS123', releasePoint: null, stand: 'C30' }} />);
    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('');
    expect(screen.queryByText('STAND', { selector: 'span' })).not.toBeInTheDocument();

    // Other stands fall back to ATC's release point.
    rerender(<D1Chart {...props} taxiClearance={{ callsign: 'SAS456', releasePoint: 'T3', stand: 'C30' }} />);
    expect(screen.getByLabelText('RELEASE POINT')).toHaveValue('T3');
    expect(screen.getByText('ATC', { selector: 'span' })).toBeInTheDocument();
  });

  it('marks the departure runway in yellow as the destination', () => {
    render(<D1Chart isOpen onClose={vi.fn()} airport="EKCH" runway="22R" procedure="ODON1C" arrival={false} taxiClearance={{ callsign: 'SAS123', mode: 'departure', releasePoint: 'Z3', stand: 'A30', runway: '22R' }} />);
    fireEvent.click(screen.getByRole('button', { name: 'TAXI CLEARANCE' }));
    const map = screen.getByRole('img', { name: 'Taxi route' });
    expect(map.querySelectorAll('polyline[stroke="#ffd84a"]').length).toBeGreaterThan(0);
    expect(within(map).getByText('RWY 22R')).toBeInTheDocument();
    expect(screen.getByLabelText('DEP RWY')).toHaveValue('22R');
    expect(screen.getByText('FLIGHT', { selector: 'span' })).toBeInTheDocument();
    // The view frames the release point and runway once, and keeps that zoom while the clearance grows.
    const viewBox = () => screen.getByRole('img', { name: 'Taxi route' }).getAttribute('viewBox');
    const framed = viewBox();
    for (const value of ['K', 'KZ', 'KZA', 'KZAx30A4']) {
      fireEvent.change(screen.getByRole('textbox', { name: 'TAXI CLEARANCE' }), { target: { value } });
      expect(viewBox()).toBe(framed);
    }

    // The assigned runway written first is the destination, not the start of the taxi route.
    fireEvent.change(screen.getByRole('textbox', { name: 'TAXI CLEARANCE' }), { target: { value: '22RKZAx30A4' } });
    expect(screen.getByLabelText('Clearance parts')).toHaveTextContent('Z3›K›Z›A›X 30›A4›HS 22R');
    expect(screen.getByText('Route OK.')).toBeInTheDocument();
  });

  it('draws arrivals from the runway exit to the assigned stand', () => {
    const props = { isOpen: true, onClose: vi.fn(), airport: 'EKCH', runway: '22L', procedure: 'TUDLO', arrival: true } as const;
    const { rerender } = render(<D1Chart {...props} taxiClearance={{ callsign: 'SAS676', mode: 'arrival', releasePoint: null, stand: 'F98', runway: '22L' }} />);
    fireEvent.click(screen.getByRole('button', { name: 'TAXI CLEARANCE' }));

    expect(screen.queryByLabelText('RELEASE POINT')).not.toBeInTheDocument();
    expect(screen.queryByText('COMMON ROUTES')).not.toBeInTheDocument();
    expect(screen.getByLabelText('LDG RWY')).toHaveValue('22L');
    expect(screen.getByLabelText('STAND')).toHaveValue('F98');
    // The standard routing for 22L is drawn as a cyan preliminary route, not written in the field.
    const cyanPieces = () => screen.getByRole('img', { name: 'Taxi route' }).querySelectorAll('polyline[stroke="#22d3ee"]').length;
    expect(screen.getByRole('textbox', { name: 'TAXI CLEARANCE' })).toHaveValue('');
    expect(screen.getByText('PRELIMINARY')).toBeInTheDocument();
    expect(screen.getByLabelText('Clearance parts')).toHaveTextContent('RWY 22L›STAND F98');
    expect(cyanPieces()).toBeGreaterThan(0);

    fireEvent.change(screen.getByRole('textbox', { name: 'TAXI CLEARANCE' }), { target: { value: 'V1V' } });
    expect(screen.queryByText('PRELIMINARY')).not.toBeInTheDocument();
    expect(cyanPieces()).toBe(0);
    expect(screen.getByText('Route OK. Taxi to stand F98.')).toBeInTheDocument();
    expect(screen.getByLabelText('Clearance parts')).toHaveTextContent('RWY 22L›V1›V›STAND F98');
    expect(within(screen.getByRole('img', { name: 'Taxi route' })).getByText('STAND F98')).toBeInTheDocument();

    // The flight's stand changes: followed until the pilot edits it.
    rerender(<D1Chart {...props} taxiClearance={{ callsign: 'SAS676', mode: 'arrival', releasePoint: null, stand: 'F96', runway: '22L' }} />);
    expect(screen.getByLabelText('STAND')).toHaveValue('F96');
    fireEvent.change(screen.getByLabelText('STAND'), { target: { value: 'f94' } });
    rerender(<D1Chart {...props} taxiClearance={{ callsign: 'SAS676', mode: 'arrival', releasePoint: null, stand: 'F92', runway: '22L' }} />);
    expect(screen.getByLabelText('STAND')).toHaveValue('F94');
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

  it('shows airport, assigned arrival and runway-matched approach charts', async () => {
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
    expect(await screen.findByTitle('Ground movement chart')).toHaveAttribute('src', 'https://charts.example.test/taxi-chart.pdf');
  });

  it('opens on the taxi clearance on the ground and on the procedure when airborne', () => {
    const props = { onClose: vi.fn(), airport: 'EKCH', runway: '22L', procedure: 'TUDLO', arrival: true, taxiClearance: { callsign: 'SAS676', mode: 'arrival' as const, releasePoint: null, stand: 'F98', runway: '22L' } };
    const pressed = () => screen.getAllByRole('button', { pressed: true }).map((button) => button.getAttribute('aria-label'));
    const { rerender } = render(<D1Chart isOpen {...props} airborne={false} />);
    expect(pressed()).toEqual(['TAXI CLEARANCE']);
    expect(screen.getByRole('img', { name: 'Taxi route' })).toBeInTheDocument();

    // Reopened while airborne: back to the procedure charts.
    rerender(<D1Chart isOpen={false} {...props} airborne />);
    rerender(<D1Chart isOpen {...props} airborne />);
    expect(pressed()).toEqual(['ASSIGNED STAR 0']);
    expect(screen.queryByRole('img', { name: 'Taxi route' })).not.toBeInTheDocument();

    // Without a taxi clearance (other airports) the procedure stays the default.
    rerender(<D1Chart isOpen={false} {...props} taxiClearance={undefined} />);
    rerender(<D1Chart isOpen {...props} taxiClearance={undefined} />);
    expect(pressed()).toEqual(['ASSIGNED STAR 0']);
  });

  it('lists the taxi clearance after the chart sections', () => {
    render(<D1Chart isOpen onClose={vi.fn()} airport="EKCH" runway="22L" procedure="TUDLO" arrival taxiClearance={{ callsign: 'SAS676', mode: 'arrival', releasePoint: null, stand: 'F98', runway: '22L' }} />);
    const sections = Array.from(document.querySelectorAll('button[aria-pressed]'), (button) => button.getAttribute('aria-label'));
    expect(sections).toEqual(['AIRPORT / TAXI 0', 'ASSIGNED STAR 0', 'APPROACH 0', 'TAXI CLEARANCE']);
  });

  it('uses the selected stand, runway, and SID briefing assets', () => {
    render(<D1Brief isOpen onClose={vi.fn()} stand="A12" sid="NEXEN2A" runway="22R" />);

    expect(screen.getByRole('dialog', { name: 'Departure briefing' })).toBeInTheDocument();
    expect(screen.getByAltText('Pushback readiness').getAttribute('src')).toContain('.webp');

    fireEvent.click(screen.getByRole('button', { name: 'Go to Pushback from A12' }));
    expect(screen.getByAltText('Pushback guidance for stand A12').getAttribute('src')).toContain('A12-A17');
    expect(screen.getByText(/A12: expect Y1/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Go to Initial taxi: 22R' }));
    expect(screen.getByAltText('Initial taxi guidance for runway 22R').getAttribute('src')).toContain('TAXI-CLEARANCE');

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

  it.each([['22L', 'A12'], ['04L', 'F98'], ['22L', 'TBA']])('adds the taxi clearance slide before On stand for runway %s, stand %s', (runway, stand) => {
    render(<D1ArrivalBrief isOpen onClose={vi.fn()} stand={stand} star="TUDLO4C" runway={runway} holdingFix="" holdingDetail="" terminalFix="" arrivalHeading="" />);
    const slides = screen.getAllByRole('button', { name: /^Go to / }).map((button) => button.getAttribute('aria-label') ?? button.textContent);
    const at = slides.findIndex((name) => name?.includes('Follow your taxi route progressively!'));
    expect(at).toBeGreaterThan(-1);
    expect(slides[at + 1]).toContain('On stand');
    expect(slides[at - 1]).toMatch(/Taxi to stand|Landing and runway vacation/);
    fireEvent.click(screen.getByRole('button', { name: 'Go to Follow your taxi route progressively!' }));
    expect(screen.getByAltText('Taxi clearance in the EFB charts').getAttribute('src')).toContain('taxi-clearance');
    expect(screen.getByText(/navigate to "Taxi Clearance"/)).toBeInTheDocument();
  });

  it.each(['22R', '04R'])('shows the taxi clearance image and hint on the initial taxi slide for %s', (runway) => {
    render(<D1Brief isOpen onClose={vi.fn()} stand="A12" sid="NEXEN2A" runway={runway} />);
    fireEvent.click(screen.getByRole('button', { name: `Go to Initial taxi: ${runway}` }));
    expect(screen.getByAltText(`Initial taxi guidance for runway ${runway}`).getAttribute('src')).toContain('TAXI-CLEARANCE');
    expect(screen.getByText(/you will see the pane of "Taxi Clearance"/)).toBeInTheDocument();
  });

  it('reminds pilots about the GSX profile and the B3 departure on 04R', () => {
    render(<D1Brief isOpen onClose={vi.fn()} stand="A12" sid="NEXEN2A" runway="04R" />);
    expect(screen.getByText('Make sure you have latest GSX Profile for EKCH!! See Options in your EFB for more')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Go to Taxi with EKCH_TWR: 04R' }));
    const paragraphs = screen.getByRole('dialog').querySelectorAll('.efb-briefing-page p');
    expect(paragraphs[0]).toHaveTextContent('All WTC/M or smaller are expected to depart from B3 unless otherwise instructed!');
    expect(paragraphs[1]).toHaveTextContent('holding point B1-B3');
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

  it('starts at the current TOBT, steps on five-minute intervals, and resets when reopened', () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(new Date('2026-06-15T23:58:00Z'));
      const props = { onClose: vi.fn(), currentTobt: '1425Z', currentCtot: 'NIL', onUpdate: vi.fn() };
      const { rerender } = render(<D2CDMDialog isOpen {...props} />);
      expect(screen.getByLabelText('New TOBT (UTC)')).toHaveValue('1425Z');
      fireEvent.click(screen.getByRole('button', { name: 'Increase TOBT by 5 minutes' }));
      expect(screen.getByLabelText('New TOBT (UTC)')).toHaveValue('1430Z');
      fireEvent.click(screen.getByRole('button', { name: 'Decrease TOBT by 5 minutes' }));
      expect(screen.getByLabelText('New TOBT (UTC)')).toHaveValue('1425Z');
      rerender(<D2CDMDialog isOpen={false} {...props} />);
      vi.setSystemTime(new Date('2026-06-16T00:10:00Z'));
      rerender(<D2CDMDialog isOpen {...props} />);
      expect(screen.getByLabelText('New TOBT (UTC)')).toHaveValue('1425Z');
    } finally {
      vi.useRealTimers();
    }
  });

  it('rounds off-grid times in the arrow direction and rounds NOW shortcuts up across midnight', () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(new Date('2026-06-15T23:58:30Z'));
      render(<D2CDMDialog isOpen onClose={vi.fn()} currentTobt="2358" currentCtot="NIL" onUpdate={vi.fn()} />);
      const input = screen.getByLabelText('New TOBT (UTC)');
      expect(input).toHaveValue('2358Z');
      fireEvent.click(screen.getByRole('button', { name: 'Increase TOBT by 5 minutes' }));
      expect(input).toHaveValue('0000Z');
      fireEvent.click(screen.getByRole('button', { name: 'Decrease TOBT by 5 minutes' }));
      expect(input).toHaveValue('2355Z');
      fireEvent.change(input, { target: { value: '1203Z' } });
      fireEvent.click(screen.getByRole('button', { name: 'Decrease TOBT by 5 minutes' }));
      expect(input).toHaveValue('1200Z');
      fireEvent.click(screen.getByRole('button', { name: 'SET TOBT TO NOW + 5 MIN' }));
      expect(input).toHaveValue('0005Z');
      fireEvent.click(screen.getByRole('button', { name: 'SET TOBT TO NOW + 15 MIN' }));
      expect(input).toHaveValue('0015Z');
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

    fireEvent.change(input, { target: { value: '1432Z' } });
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    expect(screen.getByRole('alert')).toHaveTextContent('five-minute increments');
    expect(onUpdate).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { value: '1430Z' } });
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith('1430Z'));
    expect(await screen.findByRole('alert')).toHaveTextContent('CDM unavailable');
    expect(onClose).not.toHaveBeenCalled();
  });
});

describe('EFB options', () => {
  it('loads the declared scenery, saves a new tick and keeps exactly one ticked', async () => {
    const saveScenery = vi.fn().mockResolvedValue({});
    render(<D1OptionsDialog isOpen onClose={vi.fn()} loadScenery={vi.fn().mockResolvedValue('simnord')} saveScenery={saveScenery} />);
    const sceneries = within(screen.getByRole('radiogroup', { name: 'AIRPORT SCENERY' })).getAllByRole('radio');
    expect(sceneries.map((radio) => radio.textContent?.replace('✓', ''))).toEqual(['FlyTampa Copenhagen', 'SimNord Copenhagen', 'Default']);
    await waitFor(() => expect(screen.getByRole('radio', { name: /SimNord Copenhagen/ })).toHaveAttribute('aria-checked', 'true'));

    fireEvent.click(screen.getByRole('radio', { name: /FlyTampa Copenhagen/ }));
    expect(saveScenery).toHaveBeenCalledWith('flytampa');
    expect(screen.getByRole('radio', { name: /FlyTampa Copenhagen/ })).toHaveAttribute('aria-checked', 'true');
    expect(screen.getByRole('radio', { name: /SimNord Copenhagen/ })).toHaveAttribute('aria-checked', 'false');
  });

  it('reverts the tick and says so when saving fails', async () => {
    render(<D1OptionsDialog isOpen onClose={vi.fn()} loadScenery={vi.fn().mockResolvedValue(null)} saveScenery={vi.fn().mockRejectedValue(new Error('down'))} />);
    const defaultScenery = screen.getByRole('radio', { name: /Default/ });
    await waitFor(() => expect(defaultScenery).toBeEnabled());
    fireEvent.click(defaultScenery);
    expect(await screen.findByText('Could not save your scenery choice. Try again.')).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: /Default/ })).toHaveAttribute('aria-checked', 'false');
  });

  it('waits for the initial scenery choice and serializes saves with rollback', async () => {
    let finishLoading!: (scenery: string) => void;
    const load = new Promise<string>((resolve) => { finishLoading = resolve; });
    let rejectFirstSave!: (error: Error) => void;
    const firstSave = new Promise<void>((_, reject) => { rejectFirstSave = reject; });
    const saveScenery = vi.fn().mockReturnValueOnce(firstSave).mockResolvedValueOnce(undefined);
    render(<D1OptionsDialog isOpen onClose={vi.fn()} loadScenery={() => load} saveScenery={saveScenery} />);
    const defaultScenery = screen.getByRole('radio', { name: /Default/ });
    const simNord = screen.getByRole('radio', { name: /SimNord Copenhagen/ });
    const flyTampa = screen.getByRole('radio', { name: /FlyTampa Copenhagen/ });

    expect(flyTampa).toBeDisabled();
    fireEvent.click(flyTampa);
    expect(saveScenery).not.toHaveBeenCalled();
    await act(async () => finishLoading('default'));
    expect(defaultScenery).toHaveAttribute('aria-checked', 'true');

    fireEvent.click(simNord);
    expect(simNord).toHaveAttribute('aria-checked', 'true');
    expect(flyTampa).toBeDisabled();
    fireEvent.click(flyTampa);
    expect(saveScenery.mock.calls).toEqual([['simnord']]);
    await act(async () => rejectFirstSave(new Error('down')));
    expect(defaultScenery).toHaveAttribute('aria-checked', 'true');

    fireEvent.click(flyTampa);
    await waitFor(() => expect(flyTampa).toBeEnabled());
    expect(flyTampa).toHaveAttribute('aria-checked', 'true');
    expect(saveScenery.mock.calls).toEqual([['simnord'], ['flytampa']]);
  });

  it('waits for a pending scenery save before reloading on reopen', async () => {
    let finishSave!: () => void;
    const save = new Promise<void>((resolve) => { finishSave = resolve; });
    const props = {
      onClose: vi.fn(),
      loadScenery: vi.fn().mockResolvedValueOnce('default').mockResolvedValueOnce('flytampa'),
      saveScenery: vi.fn().mockReturnValue(save),
    };
    const { rerender } = render(<D1OptionsDialog {...props} isOpen />);
    const flyTampa = () => screen.getByRole('radio', { name: /FlyTampa Copenhagen/ });
    await waitFor(() => expect(flyTampa()).toBeEnabled());
    fireEvent.click(flyTampa());
    rerender(<D1OptionsDialog {...props} isOpen={false} />);
    rerender(<D1OptionsDialog {...props} isOpen />);
    expect(flyTampa()).toBeDisabled();
    expect(props.loadScenery).toHaveBeenCalledTimes(1);

    await act(async () => finishSave());
    expect(props.loadScenery).toHaveBeenCalledTimes(2);
    expect(flyTampa()).toBeEnabled();
    expect(flyTampa()).toHaveAttribute('aria-checked', 'true');
  });

  it('links each scenery to its product page and GSX profile', () => {
    render(<D1OptionsDialog isOpen onClose={vi.fn()} />);
    expect(screen.getByRole('link', { name: 'Buy FlyTampa Copenhagen' })).toHaveAttribute('href', 'https://www.flytampa.org/ekch.html');
    expect(screen.getByRole('link', { name: 'Buy SimNord Copenhagen' })).toHaveAttribute('href', 'https://www.simnord.com/en/product/ekch-copenhagen-airport-msfs-2020-2024/');
    expect(screen.getByRole('link', { name: 'GSX profile for FlyTampa Copenhagen' })).toHaveAttribute('href', 'https://flightsim.to/addon/97152/flytampa-ekch-gsx-profile-official-vatsca-publication');
    expect(screen.getByRole('link', { name: 'GSX profile for SimNord Copenhagen' })).toHaveAttribute('href', 'https://flightsim.to/addon/114611/simnord-ekch-gsx-profile-official-vatsca-publication');
    expect(screen.queryByRole('link', { name: /Default/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Traffic Board' })).not.toBeInTheDocument();
  });
});
