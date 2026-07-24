import { useEffect, useMemo, useState } from 'react';
import { getChartFoxAccessToken, isChartFoxConfigured, startChartFoxAuthorization } from '@/lib/chartfox-auth';

const chartFoxApiUrl = 'https://api.chartfox.org/v2';
const airportChartOverviewCache = new Map<string, ChartOverview[]>();
const chartDetailCache = new Map<string, ChartData>();

interface ChartMeta {
  type_key?: string;
  value?: string[];
}

interface ChartOverview {
  id: string;
  parent_id?: string | null;
  name: string;
  type_key: string;
  meta?: ChartMeta[] | null;
}

interface ChartData extends ChartOverview {
  url?: string;
  view_url?: string;
  files?: Array<{ url?: string }> | null;
  allows_iframe?: boolean | null;
  requires_preauth?: boolean | null;
}

interface D1ChartProps {
  isOpen: boolean;
  onClose: () => void;
  airport: string;
  runway: string;
  procedure: string;
  arrival: boolean;
}

function normalise(value: string) {
  return value.toUpperCase().replace(/[^A-Z0-9]/g, '');
}

function extractChartRecords(value: unknown): ChartOverview[] {
  if (Array.isArray(value)) return value.flatMap(extractChartRecords);
  if (value === null || typeof value !== 'object') return [];
  const record = value as Record<string, unknown>;
  const chart = typeof record.id === 'string' && typeof record.type_key === 'string' ? record as unknown as ChartOverview : null;
  return [
    ...(chart ? [chart] : []),
    ...Object.values(record).flatMap(extractChartRecords),
  ];
}

function chartMatchesProcedure(chart: ChartOverview, procedure: string, type: 'SID' | 'STAR') {
  if (chart.type_key.toUpperCase() !== type) return false;
  const expected = normalise(procedure);
  if (!expected) return false;
  if (normalise(chart.name).includes(expected)) return true;
  return chart.meta?.some((meta) => meta.type_key === 'ProcedureIdent' && meta.value?.some((value) => normalise(value) === expected)) ?? false;
}

function chartMatchesRunway(chart: ChartOverview, runway: string, type: 'SID' | 'STAR') {
  if (chart.type_key.toUpperCase() !== type) return false;
  const expected = normalise(runway);
  if (!expected) return false;
  const runwayNumber = expected.replace(/[LRC]$/, '');
  const matchesRunwayNumber = chart.meta?.some((meta) => meta.type_key === 'Runways' && meta.value?.some((value) => {
    const chartRunway = normalise(value);
    return chartRunway === expected || chartRunway.replace(/[LRC]$/, '') === runwayNumber;
  })) ?? false;
  if (!matchesRunwayNumber) return false;

  // ChartFox may store only the runway number in metadata (e.g. "22"), while
  // the chart name distinguishes parallel runways ("22 L" and "22 R").
  const namedDesignator = ['L', 'R', 'C'].find((side) => normalise(chart.name).includes(`${runwayNumber}${side}`));
  return !namedDesignator || namedDesignator === expected.slice(-1);
}

function chartPageNumber(chart: ChartOverview) {
  const match = chart.name.match(/(?:-|PAGE\s+)(\d+)\s*$/i);
  return match ? Number(match[1]) : Number.POSITIVE_INFINITY;
}

export default function D1Chart({ isOpen, onClose, airport, runway, procedure, arrival }: D1ChartProps) {
  const [chartPageOverviews, setChartPageOverviews] = useState<ChartOverview[]>([]);
  const [loadedChart, setLoadedChart] = useState<ChartData | null>(null);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [loadingOverview, setLoadingOverview] = useState(false);
  const [loadingChart, setLoadingChart] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const chartType = arrival ? 'STAR' : 'SID';
  const validAirport = airport !== 'NIL' ? airport.trim().toUpperCase() : '';
  const validProcedure = procedure !== 'NIL' ? procedure.trim().toUpperCase() : '';
  const selectedChartOverview = chartPageOverviews[selectedIndex];
  const cachedSelectedChart = selectedChartOverview ? chartDetailCache.get(selectedChartOverview.id) : null;
  const selectedChart = cachedSelectedChart ?? (loadedChart?.id === selectedChartOverview?.id ? loadedChart : null);
  const hasAccessToken = Boolean(getChartFoxAccessToken());

  useEffect(() => {
    if (!isOpen || !hasAccessToken || !validAirport || !validProcedure) return;
    const controller = new AbortController();

    const request = async () => {
      setLoadingOverview(true);
      setError(null);
      setChartPageOverviews([]);
      setLoadedChart(null);
      setSelectedIndex(0);
      const token = getChartFoxAccessToken();
      if (!token) return;
      const headers = { Authorization: `Bearer ${token}` };
      let chartRecords = airportChartOverviewCache.get(validAirport);
      if (!chartRecords) {
        const overviewResponse = await fetch(`${chartFoxApiUrl}/airports/${encodeURIComponent(validAirport)}/charts/grouped`, { headers, signal: controller.signal });
        const overviewBody = await overviewResponse.json().catch(() => null) as { data?: Record<string, ChartOverview[]>; message?: string } | null;
        if (!overviewResponse.ok) throw new Error(overviewBody?.message || `ChartFox request failed (${overviewResponse.status}).`);
        chartRecords = [...new Map(extractChartRecords(overviewBody?.data).map((chart) => [chart.id, chart])).values()];
        airportChartOverviewCache.set(validAirport, chartRecords);
      }
      const chartsForType = chartRecords
        .filter((chart) => chart.type_key.toUpperCase() === chartType);
      const procedureCharts = chartsForType.filter((chart) => chartMatchesProcedure(chart, validProcedure, chartType));
      const runwayCharts = runway !== 'NIL' ? chartsForType.filter((chart) => chartMatchesRunway(chart, runway, chartType)) : [];
      // ChartFox commonly publishes a single multi-page SID/STAR chart per runway,
      // without the individual procedure ident in its metadata. In that case, the
      // runway-tagged chart is the authoritative match for the assigned procedure.
      const matchingCharts = procedureCharts.length > 0 ? procedureCharts : runwayCharts;
      const parentChartIds = [...new Set(matchingCharts.map((chart) => chart.parent_id || chart.id))];
      const chartPageOverviews = chartRecords
        .filter((chart) => parentChartIds.includes(chart.parent_id || chart.id))
        .sort((left, right) => chartPageNumber(left) - chartPageNumber(right));
      if (!controller.signal.aborted) setChartPageOverviews(chartPageOverviews);
    };

    void request()
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : 'Unable to load ChartFox charts.');
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingOverview(false);
      });

    return () => controller.abort();
  }, [chartType, hasAccessToken, isOpen, runway, validAirport, validProcedure]);

  useEffect(() => {
    if (!isOpen || !hasAccessToken || !selectedChartOverview) return;
    const cachedChart = chartDetailCache.get(selectedChartOverview.id);
    if (cachedChart) return;

    const controller = new AbortController();
    const request = async () => {
      setLoadingChart(true);
      setLoadedChart(null);
      const token = getChartFoxAccessToken();
      if (!token) return;
      const response = await fetch(`${chartFoxApiUrl}/charts/${encodeURIComponent(selectedChartOverview.id)}`, {
        headers: { Authorization: `Bearer ${token}` },
        signal: controller.signal,
      });
      const body = await response.json().catch(() => null) as ChartData | { message?: string } | null;
      if (!response.ok || !body || !('id' in body)) throw new Error(body && 'message' in body ? body.message || 'Unable to load a ChartFox chart.' : 'Unable to load a ChartFox chart.');
      chartDetailCache.set(selectedChartOverview.id, body);
      if (!controller.signal.aborted) setLoadedChart(body);
    };

    void request()
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : 'Unable to load a ChartFox chart.');
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingChart(false);
      });

    return () => controller.abort();
  }, [hasAccessToken, isOpen, selectedChartOverview]);

  const chartContext = useMemo(() => [validAirport || null, runway !== 'NIL' ? `runway ${runway}` : null, validProcedure ? `${chartType} ${validProcedure}` : null].filter(Boolean).join(', '), [chartType, runway, validAirport, validProcedure]);
  if (!isOpen) return null;

  const connect = () => void startChartFoxAuthorization('/efb').catch((reason: unknown) => setError(reason instanceof Error ? reason.message : 'Unable to start ChartFox authorization.'));
  const chartUrl = selectedChart?.url || selectedChart?.view_url;
  const canEmbed = selectedChart && selectedChart.allows_iframe !== false && !selectedChart.requires_preauth && chartUrl;
  const loading = loadingOverview || (loadingChart && !cachedSelectedChart);

  return (
    <div className="fixed inset-0 z-[1000] flex items-center justify-center bg-black/70" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-labelledby="efb-chart-title" className="flex h-[92vh] w-[98vw] max-w-[1800px] overflow-hidden rounded-xl border-2 border-[#1D293D] bg-[#011328]" onClick={(event) => event.stopPropagation()}>
        <div className="flex min-w-0 flex-[3] flex-col border-r-[10px] border-[#1D293D] bg-[#001a2e]">
          <div className="min-h-0 flex-1 p-3">
            {loading ? (
              <div className="flex h-full flex-col items-center justify-center gap-4 text-center text-[#E0E0E0]" role="status" aria-label="Loading ChartFox chart">
                <span className="h-12 w-12 animate-spin rounded-full border-4 border-white/30 border-t-white" />
                <p>Loading ChartFox chart…</p>
              </div>
            ) : canEmbed ? (
              <iframe title={selectedChart.name} src={chartUrl} className="h-full w-full border-0 bg-white" />
            ) : (
              <div className="flex h-full items-center justify-center p-8 text-center text-[#E0E0E0]">
                <div className="max-w-lg">
                  <p>Select and connect ChartFox to display the assigned procedure chart.</p>
                  {selectedChart && !canEmbed && chartUrl && <a href={chartUrl} target="_blank" rel="noreferrer" className="mt-4 inline-block border border-white px-4 py-2 font-bold text-white">OPEN IN CHARTFOX</a>}
                </div>
              </div>
            )}
          </div>
          <div className="flex h-20 shrink-0 items-center justify-center gap-5 border-t-[10px] border-[#1D293D] bg-[#001a2e] p-[15px]">
            <button type="button" aria-label="Previous chart" disabled={selectedIndex === 0} onClick={() => setSelectedIndex((index) => Math.max(0, index - 1))} className="flex h-[50px] w-[50px] items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-white text-[28px] font-bold text-black disabled:opacity-50">←</button>
            <div aria-label={`Chart page ${chartPageOverviews.length === 0 ? 0 : selectedIndex + 1} of ${chartPageOverviews.length}`} className="min-w-20 text-center text-sm text-white">{chartPageOverviews.length === 0 ? '—' : `${selectedIndex + 1} / ${chartPageOverviews.length}`}</div>
            <button type="button" aria-label="Next chart" disabled={selectedIndex >= chartPageOverviews.length - 1} onClick={() => setSelectedIndex((index) => Math.min(chartPageOverviews.length - 1, index + 1))} className="flex h-[50px] w-[50px] items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-white text-[28px] font-bold text-black disabled:opacity-50">→</button>
          </div>
        </div>

        <div className="flex min-w-[300px] max-w-[420px] flex-1 flex-col bg-[#0d2540]">
          <div className="min-h-0 flex-1 overflow-auto p-5 text-white">
            <h2 id="efb-chart-title" className="mt-0 mb-[15px] text-[clamp(16px,2.5vh,24px)] font-bold">CHARTS</h2>
            <p className="m-0 text-[clamp(12px,1.5vh,16px)] leading-[1.6] text-[#E0E0E0]">
              {chartContext ? `Assigned ${arrival ? 'arrival' : 'departure'} procedure: ${chartContext}.` : `No assigned ${arrival ? 'arrival' : 'departure'} procedure is currently available.`}
            </p>
            {!isChartFoxConfigured() && <p role="alert" className="mt-4 text-sm text-[#ffcc80]">ChartFox is not configured for this FlightStrips deployment.</p>}
            {isChartFoxConfigured() && !hasAccessToken && <button type="button" onClick={connect} className="mt-4 border border-white bg-[#1A475F] px-3 py-2 text-sm font-bold">CONNECT CHARTFOX</button>}
            {loading && <p className="mt-4 text-sm">Loading chart…</p>}
            {error && <p role="alert" className="mt-4 text-sm text-red-200">{error}</p>}
            {!loading && hasAccessToken && validProcedure && chartPageOverviews.length === 0 && !error && <p className="mt-4 text-sm">No matching {chartType} chart was found in ChartFox.</p>}
            {selectedChartOverview && <p className="mt-4 text-sm font-bold">{selectedChartOverview.name}</p>}
            <p className="mt-4 text-xs leading-[1.5] text-[#E0E0E0]">Chart data powered by <a href="https://chartfox.org" target="_blank" rel="noreferrer" className="underline">ChartFox</a>. For flight simulation only; not for real-world navigation.</p>
            <a href="https://chartfox.org/legal/privacy" target="_blank" rel="noreferrer" className="mt-2 inline-block text-xs underline">ChartFox privacy policy</a>
          </div>
          <button type="button" className="flex h-16 shrink-0 cursor-pointer items-center justify-center border-t-[10px] border-[#1D293D] bg-white" onClick={onClose}><span className="text-[clamp(14px,2vh,20px)] font-bold text-black">CLICK TO CLOSE</span></button>
        </div>
      </div>
    </div>
  );
}
