import { useEffect, useMemo, useState } from 'react';
import { ChevronRight, Map as MapIcon, Navigation, PlaneLanding, type LucideIcon } from 'lucide-react';
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
  allows_iframe?: boolean | null;
  requires_preauth?: boolean | null;
}

interface ChartFamily {
  id: string;
  name: string;
  pages: ChartOverview[];
}

type ChartSection = 'taxi' | 'procedure' | 'approach';

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
  return [...(chart ? [chart] : []), ...Object.values(record).flatMap(extractChartRecords)];
}

function chartMatchesProcedure(chart: ChartOverview, procedure: string, type: 'SID' | 'STAR') {
  if (chart.type_key.toUpperCase() !== type) return false;
  const expected = normalise(procedure);
  if (!expected) return false;
  if (normalise(chart.name).includes(expected)) return true;
  return chart.meta?.some((meta) => meta.type_key === 'ProcedureIdent' && meta.value?.some((value) => normalise(value) === expected)) ?? false;
}

function chartMatchesRunway(chart: ChartOverview, runway: string) {
  const expected = normalise(runway);
  if (!expected) return false;
  const runwayNumber = expected.replace(/[LRC]$/, '');
  const metadataMatch = chart.meta?.some((meta) => meta.type_key === 'Runways' && meta.value?.some((value) => {
    const chartRunway = normalise(value);
    return chartRunway === expected || chartRunway.replace(/[LRC]$/, '') === runwayNumber;
  })) ?? false;
  const name = normalise(chart.name);
  const namedDesignator = ['L', 'R', 'C'].find((side) => name.includes(`${runwayNumber}${side}`));

  // Published approach names consistently include the runway. Require that signal
  // when ChartFox metadata is absent so no charts for other runways leak in.
  if (!metadataMatch && !name.includes(expected)) return false;
  return !namedDesignator || namedDesignator === expected.slice(-1);
}

function chartPageNumber(chart: ChartOverview) {
  const match = chart.name.match(/(?:-|PAGE\s+)(\d+)\s*$/i);
  return match ? Number(match[1]) : Number.POSITIVE_INFINITY;
}

function chartFamilies(charts: ChartOverview[], allCharts = charts) {
  const parentIds = new Set(charts.map((chart) => chart.parent_id || chart.id));
  const byParent = new Map<string, ChartOverview[]>();
  allCharts.filter((chart) => parentIds.has(chart.parent_id || chart.id)).forEach((chart) => {
    const parentId = chart.parent_id || chart.id;
    const family = byParent.get(parentId) ?? [];
    family.push(chart);
    byParent.set(parentId, family);
  });
  return [...byParent.entries()].map(([id, pages]): ChartFamily => ({
    id,
    name: pages.find((chart) => chart.id === id)?.name ?? pages[0].name,
    pages: pages.sort((left, right) => chartPageNumber(left) - chartPageNumber(right)),
  }));
}

function isTaxiChart(chart: ChartOverview) {
  const type = chart.type_key.toUpperCase();
  const name = chart.name.toUpperCase();
  return ['APD', 'AERODROME', 'AIRPORT', 'GND', 'GROUND', 'TAXI', 'PARKING'].includes(type)
    || /\b(AERODROME|AIRPORT|GROUND|TAXI|APRON|PARKING|STAND)\b/.test(name);
}

function isApproachChart(chart: ChartOverview) {
  return ['APP', 'IAP', 'APPROACH', 'ILS', 'LOC', 'RNP', 'VOR', 'NDB'].includes(chart.type_key.toUpperCase());
}

export default function D1Chart({ isOpen, onClose, airport, runway, procedure, arrival }: D1ChartProps) {
  const [chartRecords, setChartRecords] = useState<ChartOverview[]>([]);
  const [loadedChart, setLoadedChart] = useState<ChartData | null>(null);
  const [selectedSection, setSelectedSection] = useState<ChartSection>('procedure');
  const [selectedFamilyId, setSelectedFamilyId] = useState<string | null>(null);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [loadingOverview, setLoadingOverview] = useState(false);
  const [loadingChart, setLoadingChart] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const chartType = arrival ? 'STAR' : 'SID';
  const validAirport = airport !== 'NIL' ? airport.trim().toUpperCase() : '';
  const validProcedure = procedure !== 'NIL' ? procedure.trim().toUpperCase() : '';
  const hasAccessToken = Boolean(getChartFoxAccessToken());

  const taxiFamilies = useMemo(() => chartFamilies(chartRecords.filter(isTaxiChart), chartRecords), [chartRecords]);
  const procedureFamilies = useMemo(() => {
    const chartsForType = chartRecords.filter((chart) => chart.type_key.toUpperCase() === chartType);
    const procedureCharts = chartsForType.filter((chart) => chartMatchesProcedure(chart, validProcedure, chartType));
    const runwayCharts = runway !== 'NIL' ? chartsForType.filter((chart) => chartMatchesRunway(chart, runway)) : [];
    return chartFamilies(procedureCharts.length > 0 ? procedureCharts : runwayCharts, chartRecords);
  }, [chartRecords, chartType, runway, validProcedure]);
  const approachFamilies = useMemo(() => {
    if (!arrival || runway === 'NIL') return [];
    return chartFamilies(chartRecords.filter((chart) => isApproachChart(chart) && chartMatchesRunway(chart, runway)), chartRecords);
  }, [arrival, chartRecords, runway]);
  const activeFamilies = useMemo(() => (
    selectedSection === 'taxi' ? taxiFamilies : selectedSection === 'approach' ? approachFamilies : procedureFamilies
  ), [approachFamilies, procedureFamilies, selectedSection, taxiFamilies]);
  const selectedFamily = activeFamilies.find((family) => family.id === selectedFamilyId) ?? activeFamilies[0] ?? null;
  const currentIndex = selectedFamily ? Math.min(selectedIndex, selectedFamily.pages.length - 1) : 0;
  const selectedChartOverview = selectedFamily?.pages[currentIndex];
  const cachedSelectedChart = selectedChartOverview ? chartDetailCache.get(selectedChartOverview.id) : null;
  const selectedChart = cachedSelectedChart ?? (loadedChart?.id === selectedChartOverview?.id ? loadedChart : null);

  useEffect(() => {
    if (!isOpen || !hasAccessToken || !validAirport) return;
    const controller = new AbortController();

    const request = async () => {
      setLoadingOverview(true);
      setError(null);
      setChartRecords([]);
      setLoadedChart(null);
      const token = getChartFoxAccessToken();
      if (!token) return;
      let records = airportChartOverviewCache.get(validAirport);
      if (!records) {
        const overviewResponse = await fetch(`${chartFoxApiUrl}/airports/${encodeURIComponent(validAirport)}/charts/grouped`, {
          headers: { Authorization: `Bearer ${token}` }, signal: controller.signal,
        });
        const overviewBody = await overviewResponse.json().catch(() => null) as { data?: Record<string, ChartOverview[]>; message?: string } | null;
        if (!overviewResponse.ok) throw new Error(overviewBody?.message || `ChartFox request failed (${overviewResponse.status}).`);
        records = [...new Map(extractChartRecords(overviewBody?.data).map((chart) => [chart.id, chart])).values()];
        airportChartOverviewCache.set(validAirport, records);
      }
      if (!controller.signal.aborted) setChartRecords(records);
    };

    void request().catch((reason: unknown) => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : 'Unable to load ChartFox charts.');
    }).finally(() => {
      if (!controller.signal.aborted) setLoadingOverview(false);
    });
    return () => controller.abort();
  }, [hasAccessToken, isOpen, validAirport]);

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
        headers: { Authorization: `Bearer ${token}` }, signal: controller.signal,
      });
      const body = await response.json().catch(() => null) as ChartData | { message?: string } | null;
      if (!response.ok || !body || !('id' in body)) throw new Error(body && 'message' in body ? body.message || 'Unable to load a ChartFox chart.' : 'Unable to load a ChartFox chart.');
      chartDetailCache.set(selectedChartOverview.id, body);
      if (!controller.signal.aborted) setLoadedChart(body);
    };

    void request().catch((reason: unknown) => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : 'Unable to load a ChartFox chart.');
    }).finally(() => {
      if (!controller.signal.aborted) setLoadingChart(false);
    });
    return () => controller.abort();
  }, [hasAccessToken, isOpen, selectedChartOverview]);

  if (!isOpen) return null;

  const connect = () => void startChartFoxAuthorization('/efb').catch((reason: unknown) => setError(reason instanceof Error ? reason.message : 'Unable to start ChartFox authorization.'));
  const chartUrl = selectedChart?.url || selectedChart?.view_url;
  const canEmbed = selectedChart && selectedChart.allows_iframe !== false && !selectedChart.requires_preauth && chartUrl;
  const loading = loadingOverview || (loadingChart && !cachedSelectedChart);
  const sections: Array<{ id: ChartSection; label: string; description: string; count: number; Icon: LucideIcon }> = [
    { id: 'taxi', label: 'AIRPORT / TAXI', description: 'Ground movement and apron charts', count: taxiFamilies.length, Icon: MapIcon },
    { id: 'procedure', label: `ASSIGNED ${chartType}`, description: validProcedure || 'Published procedure', count: procedureFamilies.length, Icon: Navigation },
    ...(arrival ? [{ id: 'approach' as const, label: 'APPROACH', description: runway !== 'NIL' ? `Procedures for runway ${runway}` : 'Runway-specific procedures', count: approachFamilies.length, Icon: PlaneLanding }] : []),
  ];
  const chartContext = [validAirport || null, runway !== 'NIL' ? `runway ${runway}` : null].filter(Boolean).join(', ');
  const close = () => {
    setSelectedSection('procedure');
    setSelectedFamilyId(null);
    setSelectedIndex(0);
    onClose();
  };

  return (
    <div className="fixed inset-0 z-[1000] flex items-center justify-center bg-black/70" onClick={close}>
      <div role="dialog" aria-modal="true" aria-labelledby="efb-chart-title" className="flex h-[92vh] w-[98vw] max-w-[1800px] overflow-hidden rounded-xl border-2 border-[#1D293D] bg-[#011328]" onClick={(event) => event.stopPropagation()}>
        <div className="flex min-w-0 flex-[3] flex-col border-r-[10px] border-[#1D293D] bg-[#001a2e]">
          <div className="min-h-0 flex-1 p-3">
            {loading ? <div className="flex h-full flex-col items-center justify-center gap-4 text-center text-[#E0E0E0]" role="status" aria-label="Loading ChartFox chart"><span className="h-12 w-12 animate-spin rounded-full border-4 border-white/30 border-t-white" /><p>Loading ChartFox chart…</p></div>
              : canEmbed ? <iframe title={selectedChart.name} src={chartUrl} className="h-full w-full border-0 bg-white" />
                : <div className="flex h-full items-center justify-center p-8 text-center text-[#E0E0E0]"><div className="max-w-lg"><p>{hasAccessToken ? 'Select a relevant chart to display it.' : 'Connect ChartFox to display flight charts.'}</p>{selectedChart && !canEmbed && chartUrl && <a href={chartUrl} target="_blank" rel="noreferrer" className="mt-4 inline-block border border-white px-4 py-2 font-bold text-white">OPEN IN CHARTFOX</a>}</div></div>}
          </div>
          <div className="flex h-20 shrink-0 items-center justify-center gap-5 border-t-[10px] border-[#1D293D] bg-[#001a2e] p-[15px]">
            <button type="button" aria-label="Previous chart" disabled={currentIndex === 0} onClick={() => setSelectedIndex(Math.max(0, currentIndex - 1))} className="flex h-[50px] w-[50px] items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-white text-[28px] font-bold text-black disabled:opacity-50">←</button>
            <div aria-label={`Chart page ${selectedFamily ? currentIndex + 1 : 0} of ${selectedFamily?.pages.length ?? 0}`} className="min-w-20 text-center text-sm text-white">{selectedFamily ? `${currentIndex + 1} / ${selectedFamily.pages.length}` : '—'}</div>
            <button type="button" aria-label="Next chart" disabled={!selectedFamily || currentIndex >= selectedFamily.pages.length - 1} onClick={() => setSelectedIndex(Math.min((selectedFamily?.pages.length ?? 1) - 1, currentIndex + 1))} className="flex h-[50px] w-[50px] items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-white text-[28px] font-bold text-black disabled:opacity-50">→</button>
          </div>
        </div>

        <div className="flex min-w-[300px] max-w-[420px] flex-1 flex-col bg-[#0d2540]">
          <div className="min-h-0 flex-1 overflow-auto text-white">
            <div className="border-b border-[#58708a] bg-[#102d49] px-5 py-5">
              <p className="m-0 text-[11px] font-bold tracking-[0.18em] text-[#8ed7ec]">EFB CHARTS</p>
              <h2 id="efb-chart-title" className="mt-1 mb-0 text-[clamp(18px,2.5vh,24px)] font-bold">{validAirport || 'CHARTS'}</h2>
              <p className="mt-1 mb-0 text-sm text-[#c7d9e5]">{chartContext || 'Relevant charts for this flight'}</p>
            </div>
            <div className="p-5">
              <p className="m-0 text-[11px] font-bold tracking-[0.16em] text-[#8ed7ec]">CHART SECTIONS</p>
              <div className="mt-2 grid gap-2">{sections.map((section) => {
                const isSelected = selectedSection === section.id;
                return <button key={section.id} type="button" aria-label={`${section.label} ${section.count}`} aria-pressed={isSelected} onClick={() => { setSelectedSection(section.id); setSelectedFamilyId(null); setSelectedIndex(0); }} className={`grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 border p-3 text-left transition-colors ${isSelected ? 'border-[#a9ecff] bg-[#1A475F] shadow-[inset_3px_0_0_#8ed7ec]' : 'border-[#58708a] bg-[#0a1d31] hover:border-[#8ed7ec] hover:bg-[#12314f]'}`}>
                  <span className={`flex h-9 w-9 items-center justify-center rounded-sm ${isSelected ? 'bg-[#8ed7ec] text-[#062039]' : 'bg-[#173a58] text-[#b7eafa]'}`}><section.Icon size={19} strokeWidth={2.2} /></span>
                  <span className="min-w-0"><span className="block text-sm font-bold">{section.label}</span><span className="mt-0.5 block truncate text-xs font-normal text-[#c7d9e5]">{section.description}</span></span>
                  <span className="flex items-center gap-2"><span className={`min-w-7 rounded-full px-2 py-0.5 text-center text-xs font-bold ${isSelected ? 'bg-white text-[#102d49]' : 'bg-[#254967] text-white'}`}>{section.count}</span><ChevronRight size={17} className={isSelected ? 'text-[#8ed7ec]' : 'text-[#7c9bb3]'} /></span>
                </button>;
              })}</div>
            </div>
            {!isChartFoxConfigured() && <p role="alert" className="mt-4 text-sm text-[#ffcc80]">ChartFox is not configured for this FlightStrips deployment.</p>}
            <div className="border-t border-[#58708a] px-5 py-4">
              <div className="flex items-baseline justify-between gap-3"><p className="m-0 text-[11px] font-bold tracking-[0.16em] text-[#8ed7ec]">{sections.find((section) => section.id === selectedSection)?.label ?? 'CHARTS'}</p><span className="shrink-0 text-xs text-[#c7d9e5]">{activeFamilies.length} available</span></div>
              {isChartFoxConfigured() && !hasAccessToken && <button type="button" onClick={connect} className="mt-3 border border-white bg-[#1A475F] px-3 py-2 text-sm font-bold">CONNECT CHARTFOX</button>}
              {error && <p role="alert" className="mt-3 text-sm text-red-200">{error}</p>}
              {!loading && hasAccessToken && activeFamilies.length === 0 && !error && <p className="mt-3 text-sm text-[#c7d9e5]">No relevant charts were found in ChartFox.</p>}
              <div className="mt-3 grid gap-2">{activeFamilies.map((family) => {
                const isSelected = selectedFamily?.id === family.id;
                return <button key={family.id} type="button" aria-label={family.name} aria-current={isSelected ? 'true' : undefined} onClick={() => { setSelectedFamilyId(family.id); setSelectedIndex(0); }} className={`grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border p-3 text-left transition-colors ${isSelected ? 'border-[#a9ecff] bg-[#1A475F] shadow-[inset_3px_0_0_#8ed7ec]' : 'border-[#58708a] bg-[#0a1d31] hover:border-[#8ed7ec] hover:bg-[#12314f]'}`}>
                  <span className="min-w-0"><span className="block truncate text-sm font-bold">{family.name}</span><span className="mt-1 block text-xs font-normal text-[#c7d9e5]">{family.pages.length === 1 ? 'Single chart' : `${family.pages.length} chart pages`}</span></span>
                  <span className={`rounded px-2 py-1 text-[11px] font-bold ${isSelected ? 'bg-white text-[#102d49]' : 'bg-[#254967] text-white'}`}>{isSelected ? 'VIEWING' : 'SELECT'}</span>
                </button>;
              })}</div>
            </div>
            <div className="border-t border-[#58708a] px-5 py-4 text-xs leading-[1.5] text-[#c7d9e5]"><p className="m-0">Chart data powered by <a href="https://chartfox.org" target="_blank" rel="noreferrer" className="underline hover:text-white">ChartFox</a>. For flight simulation only; not for real-world navigation.</p><a href="https://chartfox.org/legal/privacy" target="_blank" rel="noreferrer" className="mt-2 inline-block underline hover:text-white">ChartFox privacy policy</a></div>
          </div>
          <button type="button" className="flex h-16 shrink-0 cursor-pointer items-center justify-center border-t-[10px] border-[#1D293D] bg-white" onClick={close}><span className="text-[clamp(14px,2vh,20px)] font-bold text-black">CLICK TO CLOSE</span></button>
        </div>
      </div>
    </div>
  );
}
