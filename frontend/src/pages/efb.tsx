import { requireAccessToken } from "@/lib/authenticated-fetch";
import { useCallback, useEffect, useRef, useState, type CSSProperties, type FormEvent, type ReactNode } from 'react';
import { useAuth0 } from '@auth0/auth0-react';
import { getApiUrl } from '@/lib/api-url';
import { isLocalDevelopment } from '@/lib/local-development';
import { fetchVatsimAtis, parseAtisWeather } from '@/api/atis';
import { ArrowDown, Gauge, Thermometer, Wind } from 'lucide-react';
import simbriefFplImg from '../assets/efb/simbrief-fpl.png';
import fileVatsimFplImg from '../assets/efb/file-vatsim-fpl.png';
import standImage from '../assets/efb/stand-image.png';
import standArrow from '@/assets/stand-arrow.png';
import depinfoImage from '../assets/efb/depinfo-image.png';
import chartsImage from '../assets/efb/charts-image.png';
import briefingImage from '../assets/efb/briefing-image.png';
import loadingPlaceholder from '../assets/efb/loading-placeholder.png';
import loadingState from '../assets/efb/loading-state.png';
import ATISPicture from '../assets/efb/ATIS-Picture.png';
import TOBTGuidance from '../assets/efb/tobt-guidance.png';
import HOLD from '../assets/efb/HOLD.png';
import APPBRIEF from '../assets/efb/APPBRIEF.png';
import RNAVLOGO from '../assets/efb/RNAVLOGO.png';
import Headset from '../assets/efb/Headset.png';
import D1ArrivalBriefDialog from '../components/efb/dialogs/D1ArrivalBrief';
import D1BRIEFDialog from '../components/efb/dialogs/D1Brief';
import D1ChartDialog from '../components/efb/dialogs/D1Chart';
import D1DownloadsDialog from '../components/efb/dialogs/D1DownloadsDialog';
import D1STANDDialog, { type StandAvailability } from '../components/efb/dialogs/D1Stand';
import D2CDMDialog from '../components/efb/dialogs/D2CDMDialog';
import D2ATISDialog from '../components/efb/dialogs/D2ATISDialog';
import D2PDCDialog from '../components/efb/dialogs/D2PDCDialog';

type PilotState = 'OFFLINE_NO_FP' | 'OFFLINE_WITH_FP' | 'DEPARTURE' | 'ARRIVAL';
type PDCStatus = 'NOATIS' | 'NOREQ' | 'PENDING' | 'RECEIVED' | 'CONFIRMED';
type TOBTStatus = 'DEFAULT' | 'CONFIRMED' | 'ACTIVE' | 'EXPIRED';
type TSATStatus = 'DEFAULT' | 'ACTIVE' | 'EXPIRED';
type LoadState = 'LOADING' | 'READY' | 'NO_FLIGHT' | 'ERROR';

function elapsedSinceUtcTime(time: string, nowMs: number): number | null {
  if (!/^(?:[01]\d|2[0-3])[0-5]\d$/.test(time)) return null;
  const target = new Date(nowMs);
  target.setUTCHours(Number(time.slice(0, 2)), Number(time.slice(2, 4)), 0, 0);
  const day = 86_400_000;
  return ((nowMs - target.getTime() + day / 2) % day + day) % day - day / 2;
}

function atisAcknowledgementKey(flight: ApiFlight | null, source: string): string | null {
  return flight?.atis ? JSON.stringify([source, flight.callsign, flight.phase, flight.origin, flight.destination, flight.atis.code, flight.atis.text, flight.atis.frequency]) : null;
}

interface FlightDisplayData {
  callsign: string;
  stand: string;
  initialClimb: string;
  ctot: string;
  assignedRunway: string;
  depFrequency: string;
  sid: string;
  arrivalAirport: string;
  arrivalEta: string;
  arrivalRunway: string;
  terminalFix: string;
  arrivalHeading: string;
  publishedHoldingFix: string;
  publishedHoldingDetail: string;
  star: string;
  atisBetter: string;
  tobt: string;
  tobtStatus: TOBTStatus;
  tsat: string;
  tsatStatus: TSATStatus;
}

interface ApiFlight {
  groundspeed?: number | null;
  read_only?: boolean;
  flight_state?: 'online' | 'prefile';
  data_errors?: string[];
  callsign: string;
  aircraft_type?: string | null;
  origin: string;
  route?: string | null;
  stand?: string | null;
  stand_version?: number | null;
  stand_state?: string | null;
  cleared_altitude?: number | null;
  initial_climb?: string | null;
  ctot?: string | null;
  departure_frequency?: string | null;
  runway?: string | null;
  sid?: string | null;
  star?: string | null;
  arrival_guidance_expected?: boolean;
  arrival_eta?: string | null;
  terminal_fix?: string | null;
  published_heading?: number | null;
  published_holding_fix?: string | null;
  published_holding_detail?: string | null;
  destination: string;
  phase: 'DEPARTURE' | 'ARRIVAL';
  pdc_state: string;
  pdc_requires_pilot_action: boolean;
  pdc_clearance_text?: string | null;
  tobt?: string | null;
  eobt?: string | null;
  tsat?: string | null;
  atis?: { code: string; text: string[]; frequency?: string; last_updated?: string; stale: boolean } | null;
  pdc_available: boolean;
  pdc_can_submit: boolean;
  capabilities: {
    pdc: boolean;
    tobt_update: boolean;
    stand_reassignment: boolean;
    stand_initialization?: boolean;
  };
}

interface StandAvailabilityResponse {
  stands: StandAvailability[];
}

interface EfbProfile {
  callsign_lookup?: boolean;
  training_available?: boolean;
  live_mode: boolean;
  online_callsign?: string | null;
}

interface TrainingSession {
  id: number;
  name: string;
  airport: string;
}

interface ArrivalPreviewInput {
  groundspeed: string;
  callsign: string;
  origin: string;
  destination: string;
  aircraftType: string;
  route: string;
  flightLevel: string;
  stand: string;
  ctot: string;
  tobt: string;
  tsat: string;
  runway: string;
  arrivalEta: string;
  star: string;
  terminalFix: string;
  holdingFix: string;
  holdingDetail: string;
  heading: string;
}

const initialArrivalPreviewInput: ArrivalPreviewInput = {
  groundspeed: '450',
  callsign: 'SAS676',
  origin: 'EDDF',
  destination: 'EKCH',
  aircraftType: 'A320',
  route: 'MARUN Y150 TOLGI SAS MAKEL N851 GESKA Z711 MONAK',
  flightLevel: '340',
  stand: '',
  ctot: '',
  tobt: '',
  tsat: '',
  runway: '',
  arrivalEta: '',
  star: '',
  terminalFix: '',
  holdingFix: '',
  holdingDetail: '',
  heading: '',
};

const ARRIVAL_PREVIEW_KEY = 'efb-arrival-preview';

function loadArrivalPreview(): { input: ArrivalPreviewInput; onGround: boolean } {
  const defaults = { input: initialArrivalPreviewInput, onGround: false };
  if (!isLocalDevelopment) return defaults;
  try {
    const stored = window.localStorage.getItem(ARRIVAL_PREVIEW_KEY);
    if (!stored) return defaults;
    const saved: unknown = JSON.parse(stored);
    if (!saved || typeof saved !== 'object' || !('input' in saved) || !('onGround' in saved)
      || typeof saved.onGround !== 'boolean' || !saved.input || typeof saved.input !== 'object') {
      throw new Error('Invalid saved EFB preview');
    }
    const input = { ...initialArrivalPreviewInput };
    for (const key of Object.keys(input) as (keyof ArrivalPreviewInput)[]) {
      if (key === 'groundspeed' && !(key in saved.input)) {
        input.groundspeed = saved.onGround ? '0' : '450';
        continue;
      }
      if (!(key in saved.input)) throw new Error(`Missing saved EFB field: ${key}`);
      const value = Reflect.get(saved.input, key);
      if (typeof value !== 'string') throw new Error(`Invalid saved EFB field: ${key}`);
      input[key] = value;
    }
    return { input, onGround: saved.onGround };
  } catch (error) {
    console.error('Unable to restore saved EFB test flight', error);
    return defaults;
  }
}

const DEV_CALLSIGN_KEY = 'efb-test-callsign';

class ApiRequestError extends Error {
  constructor(message: string, readonly status: number) {
    super(message);
  }
}

const displayClock = (value?: string | null) => {
  const normalized = value?.trim();
  return normalized && /^\d{6}$/.test(normalized) ? normalized.slice(0, 4) : normalized;
};

const baseAircraftType = (value?: string | null) => value?.trim().toUpperCase().split('/', 1)[0] ?? '';
const displayHeading = (value?: number | null) => {
  if (typeof value !== 'number') return '';
  const heading = Math.round(value);
  return heading >= 1 && heading <= 360 ? `HDG${heading.toString().padStart(3, '0')}` : '';
};

const unavailableFlightData: FlightDisplayData = {
  callsign: '', stand: 'TBA', initialClimb: 'NIL', ctot: 'NIL', assignedRunway: 'NIL',
  depFrequency: 'NIL', sid: 'NIL', arrivalAirport: 'NIL', arrivalEta: '',
  arrivalRunway: 'NIL', terminalFix: '', arrivalHeading: '', publishedHoldingFix: '', publishedHoldingDetail: '', star: 'NIL', atisBetter: '',
  tobt: 'NIL', tobtStatus: 'DEFAULT', tsat: 'NIL', tsatStatus: 'DEFAULT',
};

type DialogType = 'D1ARRBRIEF' | 'D1BRIEF' | 'D1CHART' | 'D1DOWNLOADS' | 'D1STAND' | 'D2CDM' | 'D2ATIS' | 'D2PDC' | null;
type BoxType = 'L2' | 'M2' | 'R2' | 'L3' | 'M3' | 'R3';

const arrivalTileInfo: Record<BoxType, string[]> = {
  L2: [
    'We always recommend Navigraph charts. Otherwise, connect to ChartFox with your VATSIM account to access the relevant charts for your arrival.',
    'There is NO excuse for not having charts anymore.',
  ],
  M2: [
    'Check the information you see here CAREFULLY.',
    'These are the four most important factors when planning your way into EKCH.',
    'What is my STAR, and do I have the correct version in my FMC? What approach can I expect? Where and how do I fly the published holding?',
    'A REMINDER!!! You need to maintain the heading after the last point on the STAR. DO NOT turn inbound!!!',
  ],
  R2: [
    'Being properly prepared for your arrival is crucial.',
    'Here is all you need to know for your specific arrival in abbreviated form.',
  ],
  L3: [
    'As you are inbound to Copenhagen, you may have a CTOT.',
    'It is VITAL that you adhere to that CTOT. You can update your TOBT here for your local ATC.',
    'If you are flying UNCONTROLLED, it is YOUR job to adhere to the CTOT. If not, we will delay you en route! This creates extra work for us and is highly undesirable.',
  ],
  M3: [
    'Click here to get the latest ATIS, to know your crosswind, your QNH and the ATIS letter.',
    'This letter must, alongside your A/C type, be reported to APP on first contact.',
  ],
  R3: [
    'This shows your arrival stand assigned by ATC. We do our best to assign this, subject to whether your fellow pilots adhere to their stand assignments.',
    'If you want to increase your chance of getting this stand, or would like another stand, click to request a stand.',
    "Please select an available and suitable stand for your flight. That's everything in green.",
  ],
};

const departureTileInfo: Record<BoxType, string[]> = {
  L2: [
    'We have assigned you a stand! This complies with ALL conditions and fits our traffic flow!',
    'PLEASE USE THIS STAND IF POSSIBLE.',
    'If you want another one, click here to reserve a stand, which will then be blocked for you. Please choose one that is available and suitable for your flight.',
  ],
  M2: [
    'Check the information you see here CAREFULLY.',
    'These are the four most important factors when planning your way out of EKCH.',
    'SID, initial climb and CTOTs are all important.',
    'But make sure you understand!! You must call Departure yourself after take-off. TWR will NOT give you the frequency; they will just say "bye bye" with the take-off clearance.',
    'Once you pass 1000 ft, contact Departure yourself, please!!',
  ],
  R2: [
    'We always recommend Navigraph charts. Otherwise, connect to ChartFox with your VATSIM account to access the relevant charts for your departure.',
    'There is NO excuse for not having charts anymore.',
  ],
  L3: [
    'Find the latest ATIS here.',
    'You can request your PDC directly through this service, but you must view the ATIS first to ensure you have the latest weather before requesting clearance.',
  ],
  M3: [
    'We need to know when you are ready to go!',
    'We have made it easy for you. Click UPDATE TOBT to select the 5-minute window in which you plan to be ready for start.',
    'Then, when this time has come (+/- 5 min), you must call us for your startup. We will then find a place in the queue for you.',
  ],
  R3: [
    'Being properly prepared for your departure is crucial.',
    'Here is all you need to know for your specific departure in abbreviated form.',
  ],
};

function ArrivalTileContent({ title, info, onInfoHoverChange, children }: { title: string; info?: string[]; onInfoHoverChange: (hovered: boolean) => void; children: ReactNode }) {
  const [infoOpen, setInfoOpen] = useState(false);
  const infoButtonRef = useRef<HTMLButtonElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const closeButtonHoveredRef = useRef(false);
  const infoLabel = title.endsWith('information') ? title : `${title} information`;
  const infoBorderClassName = 'border-2 border-[#FF9800]';
  const infoButtonClassName = 'flex size-[clamp(24px,2.8vh,34px)] cursor-pointer items-center justify-center rounded-full border-[#FF9800] bg-[#011328]/95 font-serif text-[clamp(16px,2vh,23px)] font-bold text-[#B8D9F0] shadow hover:bg-[#1A475F] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#43C6E7]';

  useEffect(() => {
    if (infoOpen) closeButtonRef.current?.focus();
  }, [infoOpen]);

  useEffect(() => () => onInfoHoverChange(false), [onInfoHoverChange]);

  const closeInfo = () => {
    setInfoOpen(false);
    onInfoHoverChange(closeButtonHoveredRef.current);
    infoButtonRef.current?.focus();
  };

  return (
    <>
      <div data-efb-tile-content inert={infoOpen} className="absolute inset-0 flex items-center justify-center bg-[#000109]">
        {children}
      </div>
      <div data-efb-tile-info onClick={(event) => event.stopPropagation()} onMouseEnter={() => onInfoHoverChange(true)} onMouseLeave={() => { if (!infoOpen) onInfoHoverChange(false); }} className="absolute right-0 bottom-full left-0 z-20 h-[clamp(32px,4.2vh,44px)] cursor-default">
        <button
          ref={infoButtonRef}
          type="button"
          aria-label={infoLabel}
          aria-haspopup="dialog"
          aria-expanded={infoOpen}
          tabIndex={infoOpen ? -1 : 0}
          onClick={(event) => { event.stopPropagation(); closeButtonHoveredRef.current = false; onInfoHoverChange(true); setInfoOpen(true); }}
          onMouseEnter={() => onInfoHoverChange(true)}
          onMouseLeave={() => { if (!infoOpen) onInfoHoverChange(false); }}
          className={`absolute top-1.5 right-1.5 z-20 border ${infoButtonClassName} ${infoOpen ? 'pointer-events-none opacity-0' : ''}`}
        >
          <span aria-hidden="true">i</span>
        </button>
      </div>
      {infoOpen && (
        <div
          role="dialog"
          aria-label={infoLabel}
          onMouseEnter={() => onInfoHoverChange(true)}
          onMouseLeave={() => onInfoHoverChange(false)}
          onClick={(event) => event.stopPropagation()}
          onKeyDown={(event) => {
            if (event.key === 'Escape') {
              event.stopPropagation();
              closeInfo();
            }
          }}
          className={`absolute -inset-[2px] z-30 flex origin-top-right cursor-default animate-[efb-info-expand_50ms_ease-out] flex-col rounded-none ${infoBorderClassName} bg-[#000109] p-[clamp(12px,1.8vh,24px)] text-white shadow-xl motion-reduce:animate-none`}
        >
          <div className="flex shrink-0 items-start justify-between gap-3 border-b border-[#38536E] pb-2">
            <h2 className={`pr-[clamp(24px,2.8vh,34px)] ${title === 'Stand' ? 'text-[clamp(12px,1.6vh,20px)] leading-tight' : 'text-[clamp(16px,2.4vh,30px)]'} font-bold text-[#B8D9F0]`}>{title}</h2>
            <button
              ref={closeButtonRef}
              type="button"
              aria-label={`Close ${infoLabel.toLowerCase()}`}
              onMouseEnter={() => { closeButtonHoveredRef.current = true; }}
              onMouseLeave={() => { closeButtonHoveredRef.current = false; }}
              onClick={closeInfo}
              className={`absolute top-[calc(6px-clamp(32px,4.2vh,44px))] right-1.5 border-2 ${infoButtonClassName}`}
            >
              <span aria-hidden="true">&times;</span>
            </button>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto pt-4 text-[clamp(14px,1.8vh,24px)] leading-relaxed text-slate-200">
            {info ? info.map((paragraph, index) => (
              <div key={paragraph}>
                {index > 0 && <hr className="my-3 border-white/10" />}
                <p>{paragraph}</p>
              </div>
            )) : 'Information text will be added here.'}
          </div>
        </div>
      )}
    </>
  );
}

export default function EFBPage() {
  const { getAccessTokenSilently } = useAuth0();
  const [apiFlight, setApiFlight] = useState<ApiFlight | null>(null);
  const apiFlightRef = useRef<ApiFlight | null>(null);
  const [arrivalPreview, setArrivalPreview] = useState(false);
  const [savedArrivalPreview] = useState(loadArrivalPreview);
  const [previewOnGround, setPreviewOnGround] = useState(savedArrivalPreview.onGround);
  const arrivalPreviewRef = useRef(false);
  const [arrivalPreviewOpen, setArrivalPreviewOpen] = useState(false);
  const [arrivalPreviewInput, setArrivalPreviewInput] = useState(savedArrivalPreview.input);
  const [arrivalPreviewError, setArrivalPreviewError] = useState<string | null>(null);
  const [profile, setProfile] = useState<EfbProfile | null>(null);
  const [trainingSessions, setTrainingSessions] = useState<TrainingSession[]>([]);
  const [trainingSessionsError, setTrainingSessionsError] = useState<string | null>(null);
  const [trainingSession, setTrainingSession] = useState(() => window.sessionStorage.getItem('efb-training-session') ?? '');
  const trainingSessionRef = useRef(trainingSession);
  const [devCallsign, setDevCallsign] = useState(() => window.sessionStorage.getItem(DEV_CALLSIGN_KEY) ?? '');
  const devCallsignRef = useRef(devCallsign);
  const [pilotState, setPilotState] = useState<PilotState>('OFFLINE_NO_FP');
  const [openDialog, setOpenDialog] = useState<DialogType>(null);
  const [activeBox, setActiveBox] = useState<BoxType | null>(null);
  const [hoveredBox, setHoveredBox] = useState<BoxType | null>(null);
  const [infoHovered, setInfoHovered] = useState(false);

  // PDC flow
  const [pdcStatus, setPdcStatus] = useState<PDCStatus>('NOREQ');

  const [acknowledgedAtis, setAcknowledgedAtis] = useState<string | null>(null);
  const atisSource = arrivalPreview ? 'preview' : trainingSession || 'network';
  const currentAtisKey = atisAcknowledgementKey(apiFlight, atisSource);
  const [atisTextCurrent, setAtisTextCurrent] = useState<string>('');
  const [atisLetterCurrent, setAtisLetterCurrent] = useState<string>('');
  const [previewAtisError, setPreviewAtisError] = useState<string | null>(null);
  const [blinkOn, setBlinkOn] = useState(true);
  const [nowMs, setNowMs] = useState(Date.now);
  const [actionError, setActionError] = useState<string | null>(null);
  const [loadState, setLoadState] = useState<LoadState>('LOADING');
  const [loadError, setLoadError] = useState<string | null>(null);
  const [flightStale, setFlightStale] = useState(false);
  const [standAvailability, setStandAvailability] = useState<StandAvailability[] | null>(null);
  const [standAvailabilityError, setStandAvailabilityError] = useState<string | null>(null);
  const [confirmedTobt, setConfirmedTobt] = useState<{ session: string; callsign: string; value: string } | null>(null);
  const tobtConfirmed = !arrivalPreview && confirmedTobt !== null
    && confirmedTobt.session === trainingSession
    && confirmedTobt.callsign === apiFlight?.callsign
    && confirmedTobt.value === displayClock(apiFlight?.tobt);

  const flightData: FlightDisplayData = apiFlight ? {
    ...unavailableFlightData,
    callsign: apiFlight.callsign,
    stand: apiFlight.stand?.trim() && (apiFlight.phase === 'DEPARTURE' || ['ESTIMATED', 'ASSIGNED', 'CONFIRMED'].includes(apiFlight.stand_state ?? '')) ? apiFlight.stand.trim() : 'TBA',
    initialClimb: apiFlight.initial_climb || (apiFlight.cleared_altitude ? `FL${Math.round(apiFlight.cleared_altitude / 100)}` : 'NIL'),
    ctot: displayClock(apiFlight.ctot) || 'NIL',
    assignedRunway: apiFlight.runway || 'NIL',
    depFrequency: apiFlight.departure_frequency?.trim() || 'NIL',
    sid: apiFlight.sid || 'NIL',
    star: apiFlight.star || 'NIL',
    arrivalAirport: apiFlight.destination,
    arrivalEta: displayClock(apiFlight.arrival_eta) || '',
    arrivalRunway: apiFlight.runway || 'NIL',
    terminalFix: apiFlight.terminal_fix?.trim() || '',
    arrivalHeading: displayHeading(apiFlight.published_heading),
    publishedHoldingFix: apiFlight.published_holding_fix?.trim() || '',
    publishedHoldingDetail: apiFlight.published_holding_detail?.trim() || '',
    atisBetter: apiFlight.atis?.code || '',
    tobt: displayClock(apiFlight.tobt || apiFlight.eobt) || 'NIL',
    tobtStatus: apiFlight.tobt ? 'CONFIRMED' : 'DEFAULT',
    tsat: displayClock(apiFlight.tsat) || 'NIL',
    tsatStatus: apiFlight.tsat ? 'ACTIVE' : 'DEFAULT',
  } : unavailableFlightData;

  const authorizedFetch = useCallback(async (path: string, init?: RequestInit) => {
    const token = requireAccessToken(await getAccessTokenSilently());
    const response = await fetch(getApiUrl(path), { ...init, headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}`, ...(init?.headers ?? {}) } });
    const body = await response.json().catch(() => null);
    if (!response.ok) throw new ApiRequestError(body?.error || `Request failed (${response.status})`, response.status);
    return body;
  }, [getAccessTokenSilently]);

  const efbPath = useCallback((path: string, callsign?: string) => {
    const query = new URLSearchParams();
    if (trainingSession) query.set('training_session', trainingSession);
    if (callsign !== undefined && (trainingSession || profile?.live_mode === false)) query.set('callsign', callsign.trim().toUpperCase());
    return `${path}${query.size ? `?${query}` : ''}`;
  }, [trainingSession, profile?.live_mode]);

  useEffect(() => {
    if (!profile?.training_available) return;
    let cancelled = false;
    const refresh = async () => {
      try {
        const result = await authorizedFetch('/api/efb/training-sessions') as { sessions: TrainingSession[] };
        if (!cancelled) {
          setTrainingSessions(result.sessions);
          setTrainingSessionsError(null);
        }
      } catch (error) {
        if (!cancelled) setTrainingSessionsError(error instanceof Error ? error.message : 'Unable to load Sweatbox sessions');
      }
    };
    void refresh();
    const timer = window.setInterval(() => void refresh(), 15_000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [authorizedFetch, profile?.training_available]);

  const refreshFlight = useCallback(async (selectedCallsign = devCallsignRef.current) => {
    if (arrivalPreviewRef.current) return apiFlightRef.current;
    const selectedSession = trainingSession;
    try {
      if ((trainingSession || profile?.live_mode === false) && !selectedCallsign.trim()) {
        apiFlightRef.current = null;
        setApiFlight(null);
        setPilotState('OFFLINE_NO_FP');
        setLoadState('NO_FLIGHT');
        setLoadError(null);
        setFlightStale(false);
        return null;
      }
      if (apiFlightRef.current === null) setLoadState('LOADING');
      let flight = await authorizedFetch(efbPath('/api/efb/flight', selectedCallsign)) as ApiFlight;
      if (arrivalPreviewRef.current || selectedSession !== trainingSessionRef.current) return apiFlightRef.current;
      if (!selectedSession && flight.phase === 'DEPARTURE' && !flight.read_only && flight.capabilities.stand_initialization) {
        try {
          await authorizedFetch('/api/efb/stand/open', { method: 'POST' });
          flight = await authorizedFetch(efbPath('/api/efb/flight', selectedCallsign)) as ApiFlight;
        } catch (error) {
          flight = {
            ...flight, stand: null, stand_state: 'STAND ASSIGNMENT UNAVAILABLE',
            data_errors: [...(flight.data_errors ?? []), error instanceof Error ? error.message : 'Unable to initialize stand assignment'],
            capabilities: { ...flight.capabilities, stand_initialization: false, stand_reassignment: false },
          };
        }
        if (arrivalPreviewRef.current || selectedSession !== trainingSessionRef.current) return apiFlightRef.current;
      }
      apiFlightRef.current = flight;
      setApiFlight(flight);
      setPilotState(flight.phase === 'ARRIVAL' ? 'ARRIVAL' : flight.flight_state === 'prefile' ? 'OFFLINE_WITH_FP' : 'DEPARTURE');
      setLoadState('READY');
      setLoadError(null);
      setFlightStale(false);
      setPdcStatus(!flight.atis || flight.atis.stale ? 'NOATIS' : flight.pdc_requires_pilot_action ? 'RECEIVED' : flight.pdc_state === 'CONFIRMED' ? 'CONFIRMED' : flight.pdc_state === 'REQUESTED' ? 'PENDING' : 'NOREQ');
      if (flight.atis) {
        setAtisTextCurrent(flight.atis.text.join('\n'));
        setAtisLetterCurrent(flight.atis.code);
      } else {
        setAtisTextCurrent('');
        setAtisLetterCurrent('');
      }
      return flight as ApiFlight;
    } catch (error) {
      if (arrivalPreviewRef.current || selectedSession !== trainingSessionRef.current) return apiFlightRef.current;
      if (error instanceof ApiRequestError && error.status === 404) {
        apiFlightRef.current = null;
        setApiFlight(null);
        setPilotState('OFFLINE_NO_FP');
        setLoadState('NO_FLIGHT');
        setLoadError(null);
        setFlightStale(false);
      } else if (apiFlightRef.current !== null) {
        setFlightStale(true);
        setLoadError(error instanceof Error ? error.message : 'Unable to refresh EFB data');
      } else {
        setLoadState('ERROR');
        setLoadError(error instanceof Error ? error.message : 'Unable to load EFB data');
      }
      return null;
    }
  }, [authorizedFetch, profile, trainingSession, efbPath]);

  const loadProfile = useCallback(async () => {
    if (arrivalPreviewRef.current) return;
    setLoadState('LOADING');
    setLoadError(null);
    try {
      const value = await authorizedFetch('/api/efb/me') as EfbProfile;
      setProfile(value);
      if (!value.training_available && trainingSessionRef.current) {
        trainingSessionRef.current = '';
        setTrainingSession('');
        window.sessionStorage.removeItem('efb-training-session');
      }
      if (value.online_callsign && !trainingSessionRef.current) {
        devCallsignRef.current = value.online_callsign;
        setDevCallsign(value.online_callsign);
      } else if (!arrivalPreviewRef.current && !value.live_mode && !devCallsignRef.current.trim()) {
        setLoadState('NO_FLIGHT');
      }
    } catch (error) {
      if (arrivalPreviewRef.current) return apiFlightRef.current;
      setProfile(null);
      setLoadState('ERROR');
      setLoadError(error instanceof Error ? error.message : 'Unable to load EFB profile');
    }
  }, [authorizedFetch]);

  useEffect(() => {
    const timer = window.setTimeout(() => void loadProfile(), 0);
    return () => window.clearTimeout(timer);
  }, [loadProfile]);

  useEffect(() => {
    if (profile === null) return;
    const initial = window.setTimeout(() => void refreshFlight(), 0);
    const timer = window.setInterval(() => void refreshFlight(), 15_000);
    return () => { window.clearTimeout(initial); window.clearInterval(timer); };
  }, [profile, refreshFlight]);

  const previewAtisAirport = arrivalPreview ? apiFlight?.destination : undefined;
  useEffect(() => {
    if (!previewAtisAirport) return;
    const controller = new AbortController();
    const refreshAtis = async () => {
      try {
        const atis = await fetchVatsimAtis(previewAtisAirport, false, controller.signal);
        if (controller.signal.aborted) return;
        setPreviewAtisError(null);
        setApiFlight((flight) => {
          if (!flight) return flight;
          const updated = { ...flight, atis };
          apiFlightRef.current = updated;
          return updated;
        });
        setAtisTextCurrent(atis.text.join('\n'));
        setAtisLetterCurrent(atis.code);
      } catch (error) {
        if (controller.signal.aborted) return;
        setPreviewAtisError(error instanceof Error ? error.message : 'ATIS feed unavailable');
        setApiFlight((flight) => {
          if (!flight) return flight;
          const updated = { ...flight, atis: null };
          apiFlightRef.current = updated;
          return updated;
        });
        setAtisTextCurrent('');
        setAtisLetterCurrent('');
      }
    };
    void refreshAtis();
    const timer = window.setInterval(() => void refreshAtis(), 60_000);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [previewAtisAirport]);

  useEffect(() => {
    if (openDialog !== 'D1STAND' || !apiFlight) {
      const reset = window.setTimeout(() => {
        setStandAvailability(null);
        setStandAvailabilityError(null);
      }, 0);
      return () => window.clearTimeout(reset);
    }
    let cancelled = false;
    const refreshAvailability = async () => {
      try {
        const response = await authorizedFetch(efbPath('/api/efb/stands', devCallsignRef.current)) as StandAvailabilityResponse;
        if (!cancelled) {
          setStandAvailability(response.stands);
          setStandAvailabilityError(null);
        }
      } catch (error) {
        if (!cancelled) {
          setStandAvailability(null);
          setStandAvailabilityError(error instanceof Error ? error.message : 'Stand availability unavailable');
        }
      }
    };
    void refreshAvailability();
    const timer = window.setInterval(() => void refreshAvailability(), 15_000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [apiFlight, authorizedFetch, openDialog, efbPath]);

  const submitDevCallsign = (event: FormEvent) => {
    event.preventDefault();
    setConfirmedTobt(null);
    arrivalPreviewRef.current = false;
    setArrivalPreview(false);
    const normalized = devCallsign.trim().toUpperCase();
    devCallsignRef.current = normalized;
    setDevCallsign(normalized);
    window.sessionStorage.setItem(DEV_CALLSIGN_KEY, normalized);
    void refreshFlight(normalized);
  };

  const submitArrivalPreview = (event: FormEvent) => {
    event.preventDefault();
    const callsign = arrivalPreviewInput.callsign.trim().toUpperCase();
    const origin = arrivalPreviewInput.origin.trim().toUpperCase();
    const destination = arrivalPreviewInput.destination.trim().toUpperCase();
    if (!callsign || !/^[A-Z]{4}$/.test(origin) || !/^[A-Z]{4}$/.test(destination)) {
      setArrivalPreviewError('Enter a callsign and valid four-letter origin and destination ICAO codes.');
      return;
    }
    const flightLevel = arrivalPreviewInput.flightLevel.trim();
    const heading = arrivalPreviewInput.heading.trim();
    const ctot = arrivalPreviewInput.ctot.trim().toUpperCase().replace(/Z$/, '');
    const tobt = arrivalPreviewInput.tobt.trim().toUpperCase().replace(/Z$/, '');
    const tsat = arrivalPreviewInput.tsat.trim().toUpperCase().replace(/Z$/, '');
    const isValidUtcTime = (value: string) => !value || (/^([01]\d|2[0-3])[0-5]\d$/.test(value));
    if (!/^\d+(?:\.\d+)?$/.test(arrivalPreviewInput.groundspeed) || Number(arrivalPreviewInput.groundspeed) > 1500) {
      setArrivalPreviewError('Ground speed must be 0–1500 knots.');
      return;
    }
    if ((flightLevel && (!/^\d{1,3}$/.test(flightLevel) || Number(flightLevel) > 600))
      || (heading && (!/^\d{1,3}$/.test(heading) || Number(heading) < 1 || Number(heading) > 360))
      || !isValidUtcTime(ctot) || !isValidUtcTime(tobt) || !isValidUtcTime(tsat)) {
      setArrivalPreviewError('Flight level must be 0–600, heading must be 1–360, and TOBT, TSAT, and CTOT must be valid HHMM UTC times.');
      return;
    }

    const preview: ApiFlight = {
      callsign,
      aircraft_type: arrivalPreviewInput.aircraftType.trim().toUpperCase() || null,
      origin,
      route: arrivalPreviewInput.route.trim().toUpperCase() || null,
      destination,
      phase: 'ARRIVAL',
      cleared_altitude: flightLevel ? Number(flightLevel) * 100 : null,
      ctot: ctot || null,
      tobt: tobt || null,
      eobt: tobt || null,
      tsat: tsat || null,
      stand: arrivalPreviewInput.stand.trim().toUpperCase() || null,
      stand_state: arrivalPreviewInput.stand.trim() ? 'ASSIGNED' : null,
      runway: arrivalPreviewInput.runway.trim().toUpperCase() || null,
      arrival_eta: arrivalPreviewInput.arrivalEta.trim() || null,
      star: arrivalPreviewInput.star.trim().toUpperCase() || null,
      terminal_fix: arrivalPreviewInput.terminalFix.trim().toUpperCase() || null,
      published_holding_fix: arrivalPreviewInput.holdingFix.trim().toUpperCase() || null,
      published_holding_detail: arrivalPreviewInput.holdingDetail.trim().toUpperCase() || null,
      published_heading: heading ? Number(heading) : null,
      pdc_state: '',
      pdc_requires_pilot_action: false,
      pdc_available: false,
      pdc_can_submit: false,
      atis: arrivalPreviewRef.current && apiFlightRef.current?.destination === destination ? apiFlightRef.current.atis : null,
      capabilities: { pdc: false, tobt_update: false, stand_reassignment: false },
    };

    apiFlightRef.current = preview;
    arrivalPreviewRef.current = true;
    setApiFlight(preview);
    setArrivalPreview(true);
    setArrivalPreviewOpen(false);
    setArrivalPreviewError(null);
    setPreviewAtisError(null);
    setPilotState('ARRIVAL');
    setLoadState('READY');
    setLoadError(null);
    setFlightStale(false);
    setOpenDialog(null);
    setPdcStatus('NOREQ');
    setAtisTextCurrent(preview.atis?.text.join('\n') ?? '');
    setAtisLetterCurrent(preview.atis?.code ?? '');
    setActionError(null);
  };

  useEffect(() => {
    if (!isLocalDevelopment) return;
    try {
      window.localStorage.setItem(ARRIVAL_PREVIEW_KEY, JSON.stringify({ input: arrivalPreviewInput, onGround: previewOnGround }));
    } catch (error) {
      console.error('Unable to save EFB test flight', error);
      setActionError('Unable to save the test flight in this browser');
    }
  }, [arrivalPreviewInput, previewOnGround]);

  const updateArrivalPreviewInput = (field: keyof ArrivalPreviewInput, value: string) => {
    setArrivalPreviewInput((current) => ({ ...current, [field]: value }));
    setArrivalPreviewError(null);
  };

  const exitArrivalPreview = () => {
    arrivalPreviewRef.current = false;
    setArrivalPreview(false);
    apiFlightRef.current = null;
    setApiFlight(null);
    setLoadState(profile === null ? 'LOADING' : 'NO_FLIGHT');
    setPilotState('OFFLINE_NO_FP');
    if (profile === null) void loadProfile();
    else if (profile.live_mode || devCallsignRef.current.trim()) void refreshFlight();
  };

useEffect(() => {
  const id = setInterval(() => {
    setBlinkOn((v) => !v);
    setNowMs(Date.now());
  }, 500); // 0.5s
  return () => clearInterval(id);
}, []);

  const isArrival = pilotState === 'ARRIVAL';
  const showFlightplanElements = loadState === 'READY' && pilotState !== 'OFFLINE_NO_FP';

  const arrivalGroundspeed = arrivalPreview ? Number(arrivalPreviewInput.groundspeed) : apiFlight?.groundspeed;
  const timingInhibited = isArrival && typeof arrivalGroundspeed === 'number' && arrivalGroundspeed > 60;

  useEffect(() => {
    if (timingInhibited && openDialog === 'D2CDM') {
      setOpenDialog(null);
      setActiveBox(null);
    }
  }, [timingInhibited, openDialog]);

  const updatePreviewOnGround = (onGround: boolean) => {
    setPreviewOnGround(onGround);
    updateArrivalPreviewInput('groundspeed', onGround ? '0' : '450');
    setOpenDialog(null);
    setActiveBox(null);
    setHoveredBox(null);
  };

  const departureDialogMap: Record<BoxType, DialogType> = {
    L2: 'D1STAND',
    M2: null,
    R2: 'D1CHART',
    L3: 'D2ATIS',
    M3: 'D2CDM',
    R3: 'D1BRIEF',
  };

  const arrivalDialogMap: Record<BoxType, DialogType> = {
    L2: 'D1CHART',
    M2: null,
    R2: 'D1ARRBRIEF',
    L3: 'D2CDM',
    M3: 'D2ATIS',
    R3: 'D1STAND',
  };

  const getDialogMap = (): Record<BoxType, DialogType> => {
    return isArrival ? arrivalDialogMap : departureDialogMap;
  };

  const tilePosition = (box: BoxType): BoxType => {
    if (isArrival) return box;
    return ({ L2: 'L2', M2: 'M3', R2: 'L3', L3: 'R2', M3: 'M2', R3: 'R3' } as const)[box];
  };
  const tileStyle = (box: BoxType) => {
    const position = tilePosition(box);
    return {
      top: `calc(${position.endsWith('2') ? '15%' : '57.5%'} + clamp(32px, 4.2vh, 44px))`,
      left: position.startsWith('L') ? '2.5%' : position.startsWith('M') ? '34.58%' : '66.66%',
      height: 'calc(40% - clamp(32px, 4.2vh, 44px))',
    };
  };

  const handleBoxClick = (boxType: BoxType) => {
    if (!showFlightplanElements || (timingInhibited && boxType === 'L3')) return;

    if (isArrival && boxType === 'M3') {
      handleArrivalM3AtisClick();
      return;
    }

    const dialogMap = getDialogMap();
    const dialogType = dialogMap[boxType];
    if (dialogType === 'D1STAND' && !apiFlight?.capabilities.stand_reassignment) {
      setActionError('Stand reassignment is currently unavailable');
      return;
    }
    if (dialogType === 'D2CDM' && !arrivalPreview && !apiFlight?.capabilities.tobt_update) {
      setActionError('TOBT updates are currently unavailable');
      return;
    }
    if (dialogType) {
      setOpenDialog(dialogType);
      setActiveBox(boxType);
    }
  };

  const handleCloseDialog = () => {
    setOpenDialog(null);
    setActiveBox(null);
  };

  const handleDownloadsClick = () => {
    if (!showFlightplanElements) return;
    setOpenDialog('D1DOWNLOADS');
    setActiveBox(null);
  };

  const openAtisDialog = (box: BoxType, flight: ApiFlight) => {
    setAcknowledgedAtis(atisAcknowledgementKey(flight, atisSource));
    setOpenDialog('D2ATIS');
    setActiveBox(box);
  };

  const handleAtisPrimaryClick = async (sourceBox: BoxType) => {
    if (!showFlightplanElements) return;
    setActionError(null);
    if (arrivalPreview) {
      if (!apiFlightRef.current?.atis) {
        setActionError(previewAtisError || 'ATIS is currently unavailable');
        return;
      }
      openAtisDialog(sourceBox, apiFlightRef.current);
      return;
    }
    const refreshedFlight = await refreshFlight();
    if (!refreshedFlight?.atis) {
      setActionError('ATIS is currently unavailable');
      return;
    }
    openAtisDialog(sourceBox, refreshedFlight);
  };

  const handleL3M1Click = () => {
    if (isArrival) return;
    void handleAtisPrimaryClick('L3');
  };

  const handleArrivalM3AtisClick = () => {
    if (!isArrival) return;
    void handleAtisPrimaryClick('M3');
  };

  const pdcActionsAvailable = !apiFlight?.read_only && Boolean(apiFlight?.capabilities.pdc);

  const handleL3M2Click = () => {
    if (isArrival || !showFlightplanElements) return;

    if (!pdcActionsAvailable && pdcStatus !== 'NOATIS') {
      setActionError('PDC actions are currently unavailable');
      return;
    }
    if (pdcStatus === 'NOATIS') {
      setActionError('Current ATIS is required before requesting PDC');
      return;
    }
    if (pdcStatus === 'NOREQ' && apiFlight?.pdc_can_submit) {
      setActionError(null);
      setPdcStatus('PENDING');
      void authorizedFetch('/api/pdc/request', { method: 'POST', body: JSON.stringify({ callsign: flightData.callsign, aircraft_type: baseAircraftType(apiFlight?.aircraft_type), atis: flightData.atisBetter, stand: flightData.stand === 'TBA' ? '' : flightData.stand, remarks: '' }) })
        .then(() => refreshFlight())
        .catch((error: unknown) => { setPdcStatus('NOREQ'); setActionError(error instanceof Error ? error.message : 'Unable to request PDC'); });
      return;
    }
    if (pdcStatus === 'NOREQ') {
      setActionError('PDC requests are currently unavailable');
      return;
    }

    if (pdcStatus === 'RECEIVED') {
      setOpenDialog('D2PDC');
      setActiveBox('L3');
    }
  };

  const handlePdcConfirm = async () => {
    if (!pdcActionsAvailable || !apiFlight?.pdc_requires_pilot_action) {
      throw new Error('PDC actions are currently unavailable');
    }
    await authorizedFetch('/api/pdc/acknowledge', { method: 'POST', body: JSON.stringify({ callsign: flightData.callsign }) });
    setPdcStatus('CONFIRMED');
    await refreshFlight();
  };

  const handlePdcUnable = async () => {
    if (!pdcActionsAvailable || !apiFlight?.pdc_requires_pilot_action) {
      throw new Error('PDC actions are currently unavailable');
    }
    await authorizedFetch('/api/pdc/unable', { method: 'POST', body: JSON.stringify({ callsign: flightData.callsign }) });
    setPdcStatus('NOREQ');
    await refreshFlight();
  };

  const getM1HeaderColor = () => {
    if (pilotState === 'OFFLINE_NO_FP') return 'bg-[#B63F3F]';
    if (pilotState === 'OFFLINE_WITH_FP') return 'bg-[#414141]';
    return 'bg-[#41826E]';
  };

  const getM1HeaderText = () => {
    if (loadState === 'LOADING') return 'LOADING';
    if (loadState === 'ERROR') return 'SERVICE UNAVAILABLE';
    if (pilotState === 'OFFLINE_NO_FP') return 'NO FLIGHTPLAN';
    if (pilotState === 'OFFLINE_WITH_FP') return 'OFFLINE';
    if (pilotState === 'ARRIVAL') return 'ARRIVAL';
    return 'DEPARTURE';
  };

  const getL2Title = () => {
    if (pilotState === 'ARRIVAL') return 'STAND';
    return 'STAND';
  };

  const getStandM1Color = () => {
    if (!showFlightplanElements) return 'bg-[#2E343D]';
    const state = getStandM3Text();
    if (apiFlight?.phase === 'DEPARTURE') {
      if (state === 'MISMATCH') return 'bg-[#B63F3F]';
      if (state === 'CONFIRMED' || state === 'OVERWRITTEN' || state === 'SENSED') return 'bg-[#41826E]';
      if (state === 'ASSIGNED' || state === 'SELECTED') return 'bg-[#1A475F]';
      return 'bg-[#2E343D]';
    }
    if (pilotState === 'OFFLINE_WITH_FP') return 'bg-[#1A475F]';
    return flightData.stand === 'TBA' ? 'bg-[#2E343D]' : 'bg-[#1A475F]';
  };

  const getStandM3Color = () => {
    if (!showFlightplanElements) return 'bg-[#2E343D]';
    if (apiFlight?.phase === 'DEPARTURE') return getStandM1Color();
    if (getStandM3Text() === 'SENSED') return 'bg-[#41826E]';
    if (pilotState === 'OFFLINE_WITH_FP') return 'bg-[#2E435F]';
    return flightData.stand === 'TBA' ? 'bg-[#2E343D]' : 'bg-[#1A475F]';
  };

  const getStandM3Text = () => {
    if (apiFlight?.phase === 'DEPARTURE' && apiFlight.stand_state) return apiFlight.stand_state;
    if (flightData.stand === 'TBA') return 'TBA';
    return apiFlight?.phase === 'ARRIVAL' ? apiFlight.stand_state : 'SENSED';
  };

  const standStateTextClass = (getStandM3Text()?.length ?? 0) > 14
    ? 'text-[clamp(12px,2vh,28px)]'
    : 'text-[clamp(14px,3.5vh,53px)]';

  const getAtisColor = () => {
    return currentAtisKey !== null && currentAtisKey === acknowledgedAtis && !apiFlight?.atis?.stale
      ? 'bg-[#41826E]' : 'bg-[#1A475F]';
  };

  const getAtisText = () => {
    return atisLetterCurrent ? `ATIS ${atisLetterCurrent}${apiFlight?.atis?.stale ? ' STALE' : ''}` : 'REQ ATIS';
  };

  const getClrM2Color = () => {
    if (pdcStatus === 'NOATIS') return 'bg-[#8C3838]';
    if (pdcStatus === 'NOREQ') return apiFlight?.pdc_can_submit ? 'bg-[#1A475F]' : 'bg-[#2E343D]';
    if (pdcStatus === 'PENDING') return 'bg-[#FF9800]';
    if (pdcStatus === 'RECEIVED') return blinkOn ? 'bg-[#43C6E7]' : 'bg-[#000109]';
    return 'bg-[#41826E]';
  };

  const getClrM2Text = () => {
    if (pdcStatus === 'NOATIS') return 'REQ PDC';
    if (pdcStatus === 'NOREQ') return apiFlight?.pdc_can_submit ? 'REQ PDC' : 'PDC UNAVAILABLE';
    if (pdcStatus === 'PENDING') return 'STANDBY';
    if (pdcStatus === 'RECEIVED') return 'VIEW PDC';
    return 'PDC COMPLETE';
  };

  const weather = parseAtisWeather(atisTextCurrent);
  const windDegrees = weather.windDirection !== null && /^\d+$/.test(weather.windDirection) ? Number(weather.windDirection) : null;
  const runwayHeading = /^04[LR]?$/.test(flightData.assignedRunway) ? 40 : /^22[LR]?$/.test(flightData.assignedRunway) ? 220 : 0;
  const windArrowDegrees = windDegrees === null ? null : ((windDegrees - runwayHeading) % 360 + 360) % 360;
  const weatherWidgets = [
    { label: 'WIND', icon: windDegrees === null ? Wind : ArrowDown, value: `${weather.windDirection === 'VRB' ? 'VRB ' : ''}${weather.windStrength ?? 'N/A'}` },
    { label: 'TEMPERATURE', icon: Thermometer, value: weather.temperature === null ? 'N/A' : `${weather.temperature} °C` },
    { label: 'QNH', icon: Gauge, value: weather.qnh === null ? 'N/A' : `${weather.qnh} hPa` },
  ];
  const atisWeatherWidgets = (
    <div role="group" aria-label="ATIS weather" className="absolute top-1/2 left-0 grid h-[30%] w-full grid-cols-3 items-center px-[3%] text-white">
      {weatherWidgets.map(({ label, icon: Icon, value }) => (
        <div key={label} role="group" aria-label={label} className="flex min-w-0 flex-col items-center justify-center gap-[0.5vh] bg-transparent text-center">
          <Icon role={label === 'WIND' && windDegrees !== null ? 'img' : undefined} aria-label={label === 'WIND' && windDegrees !== null ? `Wind from ${windDegrees} degrees` : undefined} aria-hidden={label !== 'WIND' || windDegrees === null ? true : undefined} style={label === 'WIND' && windArrowDegrees !== null ? { transform: `rotate(${windArrowDegrees}deg)` } : undefined} className="size-[clamp(16px,2.4vh,30px)] text-[#B8D9F0]" strokeWidth={1.5} />
          <span className="text-[clamp(8px,1.1vh,14px)] tracking-wider text-[#B8D9F0]">{label}</span>
          <span className="whitespace-nowrap text-[clamp(12px,2.2vh,32px)] leading-none font-bold">{value}</span>
        </div>
      ))}
    </div>
  );

  const tsatDisplayed = flightData.tsatStatus !== 'DEFAULT' && flightData.tsatStatus !== 'EXPIRED';
  const tsatElapsed = tsatDisplayed ? elapsedSinceUtcTime(flightData.tsat, nowMs) : null;
  const tsatOverdue = tsatElapsed !== null && tsatElapsed > 6 * 60_000;
  const ctotElapsed = elapsedSinceUtcTime(flightData.ctot, nowMs);
  const ctotActive = ctotElapsed !== null && ctotElapsed >= -6 * 60_000 && ctotElapsed <= 11 * 60_000;

  const getCdmL1Color = () => {
    if (timingInhibited) return 'bg-[#2E343D]';
    if (tsatOverdue) return 'bg-[#B63F3F]';
    if (tobtConfirmed) return 'bg-[#41826E]';
    const status = flightData.tobtStatus;
    if (status === 'DEFAULT') return 'bg-[#000109]';
    if (status === 'CONFIRMED') return 'bg-[#1A475F]';
    if (status === 'ACTIVE') return 'bg-[#41826E]';
    return 'bg-[#B63F3F]';
  };

  const getCdmR1Color = () => {
    if (timingInhibited) return 'bg-[#2E343D]';
    if (!tsatDisplayed) return 'bg-transparent';
    if (tsatOverdue) return 'bg-[#B63F3F]';
    if (tsatElapsed !== null && Math.abs(tsatElapsed) <= 5 * 60_000) return 'bg-[#41826E]';
    return 'bg-[#1A475F]';
  };

  const timingGuidance = (text: string) => (
    <div className="absolute top-[76%] left-0 w-full whitespace-nowrap text-center text-[min(1.3vh,6cqw)] leading-tight text-white">{text}</div>
  );

  const getHoverClass = (boxType: BoxType) => (
    hoveredBox === boxType && !infoHovered && showFlightplanElements && (isArrival ? ['L2', 'R2'] : ['L3', 'R3']).includes(tilePosition(boxType))
      ? 'z-[3] efb-tile-hover'
      : 'z-[1]'
  );

  const timingTile = (
    <>
      <div role="group" aria-label="TOBT" className={`${getCdmL1Color() === 'bg-[#000109]' ? '' : 'efb-action-bar'} absolute top-0 left-0 h-1/2 w-1/3 border-[clamp(5px,0.5vw,18px)] border-[#000109] [container-type:inline-size] ${getCdmL1Color()}`}>
        <div className="absolute top-[8%] left-0 w-full text-center text-[clamp(14px,3.5vh,74px)] leading-none font-normal text-white">TOBT</div>
        <div className="absolute top-[38%] left-0 w-full text-center text-[clamp(14px,5.5vh,106px)] leading-none font-bold text-white">{flightData.tobt}</div>
        {timingGuidance('call for start within +/- 5min')}
      </div>
      <div role="group" aria-label="TSAT" className={`${getCdmR1Color() === 'bg-transparent' ? '' : 'efb-action-bar'} absolute top-0 left-1/3 h-1/2 w-1/3 border-[clamp(5px,0.5vw,18px)] border-[#000109] [container-type:inline-size] ${getCdmR1Color()}`}>
        <div className="absolute top-[8%] left-0 w-full text-center text-[clamp(14px,3.5vh,74px)] leading-none font-normal text-white">TSAT</div>
        {tsatDisplayed && (
          <>
            <div className="absolute top-[38%] left-0 w-full text-center text-[clamp(14px,5.5vh,106px)] leading-none font-bold text-white">{flightData.tsat}</div>
            {timingGuidance('Expected startup time +/-5min')}
          </>
        )}
      </div>
      <div role="group" aria-label="CTOT" className={`efb-action-bar absolute top-0 left-2/3 h-1/2 w-1/3 border-[clamp(5px,0.5vw,18px)] border-[#000109] [container-type:inline-size] ${timingInhibited ? 'bg-[#2E343D]' : flightData.ctot === 'NIL' || ctotActive ? 'bg-[#41826E]' : 'bg-[#1A475F]'}`}>
        <div className="absolute top-[8%] left-0 w-full text-center text-[clamp(14px,3.5vh,74px)] leading-none font-normal text-white">CTOT</div>
        <div className="absolute top-[38%] left-0 w-full text-center text-[clamp(14px,5.5vh,106px)] leading-none font-bold text-white">{flightData.ctot}</div>
        {ctotElapsed !== null && timingGuidance('Expect takeoff within -5 to +10')}
      </div>
      <div className="absolute top-1/2 left-0 flex h-[30%] w-full items-center justify-center">
        <img src={TOBTGuidance} alt="TOBT stand guidance" className="h-full w-full scale-[1.15] object-contain" />
      </div>
      <button type="button" disabled={timingInhibited || (!arrivalPreview && !apiFlight?.capabilities.tobt_update)} onClick={(event) => { event.stopPropagation(); handleBoxClick(isArrival ? 'L3' : 'M3'); }} className={`efb-action-bar ${timingInhibited ? '' : 'efb-clickspot'} absolute top-[80%] left-0 flex h-[20%] w-full cursor-pointer items-center justify-center border-[clamp(5px,0.5vw,18px)] border-[#000109] focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-[#43C6E7] ${timingInhibited ? 'bg-[#2E343D]' : tobtConfirmed ? 'bg-[#41826E]' : 'bg-[#1A475F]'} text-[clamp(14px,3.5vh,53px)] font-bold text-white disabled:cursor-default ${tobtConfirmed && !timingInhibited ? '' : 'disabled:bg-[#2E343D]'}`}>{timingInhibited ? 'INHIBITED IN AIR' : tobtConfirmed ? 'TOBT CONFIRMED' : 'UPDATE TOBT'}</button>
    </>
  );

  return (
    <div className="relative h-screen w-screen overflow-hidden bg-[#1D293D]">
      {(profile?.live_mode === false || profile?.training_available) && (
        <form onSubmit={submitDevCallsign} className="absolute top-2 left-2 z-[50] flex gap-1.5 bg-black/70 p-1.5">
          {profile?.training_available && (
            <select aria-label="EFB data source" value={trainingSession} onChange={(event) => {
              const session = event.target.value;
              setConfirmedTobt(null);
              trainingSessionRef.current = session;
              setTrainingSession(session);
              window.sessionStorage.setItem('efb-training-session', session);
              arrivalPreviewRef.current = false;
              setArrivalPreview(false);
              apiFlightRef.current = null;
              setApiFlight(null);
              setOpenDialog(null);
              setActionError(null);
              setLoadState('LOADING');
            }} className="border border-[#43C6E7] bg-[#011328] px-2 text-white">
              <option value="">VATSIM</option>
              {trainingSessions.length === 0 && <option disabled>No active Sweatbox sessions</option>}
              {trainingSession && !trainingSessions.some(session => String(session.id) === trainingSession) && <option value={trainingSession}>Sweatbox session unavailable</option>}
              {trainingSessions.map(session => <option key={session.id} value={session.id}>{session.name} - {session.airport} (#{session.id})</option>)}
            </select>
          )}
          <input aria-label={trainingSession ? 'Sweatbox callsign' : profile?.callsign_lookup ? 'VATSIM callsign' : 'Development callsign'} value={devCallsign} onChange={(event) => setDevCallsign(event.target.value.toUpperCase())} placeholder="CALLSIGN" className="w-[130px] border border-[#43C6E7] bg-[#011328] px-2 py-[5px] font-mono font-bold text-white" />
          <button type="submit" className="cursor-pointer border border-[#43C6E7] bg-[#1A475F] px-3 py-[5px] font-bold text-white">LOAD</button>
        </form>
      )}
      {trainingSessionsError && <div role="alert" className="absolute top-14 left-2 z-[50] bg-black/90 p-2 text-red-400">{trainingSessionsError}</div>}
      {trainingSession && !arrivalPreview && (
        <div role="status" aria-label="Sweatbox training" className="absolute bottom-2 left-2 z-[50] bg-black/90 px-3 py-2 font-mono text-xs text-amber-200">
          SWEATBOX TRAINING - SESSION {trainingSession} - LOCAL TOBT / STAND UPDATES ONLY{apiFlight && ` - ${apiFlight.callsign} - ${apiFlight.origin} TO ${apiFlight.destination}`}
        </div>
      )}
      {profile?.callsign_lookup && !trainingSession && !arrivalPreview && (
        <div role="status" aria-label="VATSIM callsign lookup" className="absolute bottom-2 left-2 z-[50] bg-black/90 px-3 py-2 font-mono text-xs text-cyan-100">
          VATSIM CALLSIGN LOOKUP - READ ONLY{apiFlight && ` - ${apiFlight.callsign} - ${apiFlight.flight_state === 'prefile' ? 'PREFILED' : 'ONLINE'} - ${apiFlight.origin} TO ${apiFlight.destination}`}
        </div>
      )}
      {isLocalDevelopment && (
        arrivalPreview ? (
          <div role="status" aria-label="Simulated arrival preview" className="absolute top-2 right-2 z-[50] flex max-w-[70vw] items-center gap-2 bg-black/90 p-2 font-mono text-xs text-cyan-100 sm:text-sm">
            <div className="flex min-w-0 flex-col">
              <span className="font-bold">SIMULATED ARRIVAL — READ ONLY</span>
              <span>{apiFlight?.callsign} · {apiFlight?.origin}–{apiFlight?.destination}{apiFlight?.aircraft_type && ` · ${apiFlight.aircraft_type}`}{apiFlight?.cleared_altitude && ` · FL${Math.round(apiFlight.cleared_altitude / 100)}`}</span>
              {apiFlight?.route && <span className="break-words text-[10px] text-slate-300 sm:text-xs">{apiFlight.route}</span>}
            </div>
            <label className="flex shrink-0 items-center gap-2">
              <input type="checkbox" checked={previewOnGround} onChange={(event) => updatePreviewOnGround(event.target.checked)} />
              On ground
            </label>
            <button type="button" onClick={() => setArrivalPreviewOpen(true)} className="shrink-0 border border-cyan-300 px-2 py-1 font-bold">EDIT</button>
            <button type="button" onClick={exitArrivalPreview} className="shrink-0 border border-cyan-300 px-2 py-1 font-bold">EXIT PREVIEW</button>
          </div>
        ) : (
          <button
            type="button"
            onClick={() => { setArrivalPreviewError(null); setArrivalPreviewOpen(true); }}
            className="absolute top-2 right-2 z-[50] border border-cyan-300 bg-[#011328] px-3 py-2 font-mono font-bold text-cyan-100"
          >
            PREVIEW ARRIVAL
          </button>
        )
      )}
      {isLocalDevelopment && arrivalPreviewOpen && (
        <div className="fixed inset-0 z-[1000] flex items-center justify-center bg-black/80 p-4">
          <form
            aria-label="Simulated arrival"
            onSubmit={submitArrivalPreview}
            className="max-h-[90vh] w-full max-w-3xl overflow-y-auto border-2 border-cyan-300 bg-[#011328] p-5 font-mono text-white shadow-2xl"
          >
            <h1 className="mb-2 text-xl font-bold text-cyan-100">Simulate an arrival</h1>
            <p className="mb-4 text-sm text-slate-300">Preview-only data. No flight or controller state will be changed.</p>
            <label className="mb-4 flex items-center gap-2 text-sm text-cyan-100">
              <input type="checkbox" checked={previewOnGround} onChange={(event) => updatePreviewOnGround(event.target.checked)} />
              On ground
            </label>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
              {([
                ['callsign', 'Callsign', true],
                ['origin', 'Origin ICAO', true],
                ['destination', 'Destination ICAO', true],
                ['aircraftType', 'Aircraft type', false],
                ['flightLevel', 'Cruise flight level', false],
                ['groundspeed', 'Ground speed (knots)', true],
                ['stand', 'Stand', false],
                ['tobt', 'TOBT (HHMM UTC)', false],
                ['tsat', 'TSAT (HHMM UTC)', false],
                ['ctot', 'CTOT (HHMM UTC)', false],
                ['runway', 'Arrival runway', false],
                ['arrivalEta', 'Arrival estimate (HHMM)', false],
                ['star', 'STAR', false],
                ['terminalFix', 'Terminal fix', false],
                ['holdingFix', 'Published holding fix', false],
                ['holdingDetail', 'Holding detail', false],
                ['heading', 'Published heading', false],
              ] as const).map(([field, label, required]) => (
                <label key={field} className="flex flex-col gap-1 text-xs font-bold text-cyan-100">
                  {label}
                  <input
                    aria-label={label}
                    required={required}
                    value={arrivalPreviewInput[field]}
                    onChange={(event) => updateArrivalPreviewInput(field, event.target.value)}
                    className="min-w-0 border border-slate-500 bg-[#000109] px-2 py-2 text-sm text-white"
                  />
                </label>
              ))}
              <label className="col-span-2 flex flex-col gap-1 text-xs font-bold text-cyan-100 sm:col-span-3">
                Route
                <textarea
                  aria-label="Route"
                  rows={2}
                  value={arrivalPreviewInput.route}
                  onChange={(event) => updateArrivalPreviewInput('route', event.target.value)}
                  className="min-w-0 resize-y border border-slate-500 bg-[#000109] px-2 py-2 text-sm text-white"
                />
              </label>
            </div>
            {arrivalPreviewError && <p role="alert" className="mt-3 text-sm font-bold text-red-300">{arrivalPreviewError}</p>}
            <div className="mt-5 flex justify-end gap-2">
              <button type="button" onClick={() => setArrivalPreviewOpen(false)} className="border border-slate-400 px-4 py-2">CANCEL</button>
              <button type="submit" className="border border-cyan-300 bg-[#1A475F] px-4 py-2 font-bold">SHOW ARRIVAL</button>
            </div>
          </form>
        </div>
      )}
      <D1BRIEFDialog
        isOpen={openDialog === 'D1BRIEF'}
        onClose={handleCloseDialog}
        stand={flightData.stand}
        sid={flightData.sid}
        runway={flightData.assignedRunway}
      />
      <D1ArrivalBriefDialog
        isOpen={openDialog === 'D1ARRBRIEF'}
        onClose={handleCloseDialog}
        stand={flightData.stand}
        star={flightData.star}
        runway={flightData.arrivalRunway}
        holdingFix={flightData.publishedHoldingFix}
        holdingDetail={flightData.publishedHoldingDetail}
        terminalFix={flightData.terminalFix}
        arrivalHeading={flightData.arrivalHeading}
        expected={apiFlight?.arrival_guidance_expected}
      />
      <D1ChartDialog
        isOpen={openDialog === 'D1CHART'}
        onClose={handleCloseDialog}
        airport={isArrival ? flightData.arrivalAirport : apiFlight?.origin || 'NIL'}
        runway={isArrival ? flightData.arrivalRunway : flightData.assignedRunway}
        procedure={isArrival ? flightData.star : flightData.sid}
        arrival={isArrival}
        expected={isArrival && apiFlight?.arrival_guidance_expected}
      />
      <D1DownloadsDialog
        isOpen={openDialog === 'D1DOWNLOADS'}
        onClose={handleCloseDialog}
      />
      {openDialog === 'D1STAND' && (
        <D1STANDDialog
          isOpen
          onClose={handleCloseDialog}
          stand={flightData.stand}
          availability={standAvailability ?? undefined}
          availabilityLoading={standAvailability === null && standAvailabilityError === null}
          availabilityUnavailable={standAvailabilityError !== null}
          onRequest={async (requestedStand, allowUnsuitable) => {
            const result = await authorizedFetch(efbPath('/api/efb/stand'), { method: 'PUT', body: JSON.stringify({ stand: requestedStand, allow_unsuitable: allowUnsuitable, version: apiFlight?.stand_version ?? 0, callsign: trainingSession || profile?.live_mode === false ? devCallsignRef.current : undefined }) }) as { stand: string; stand_state: string; version: number };
            setApiFlight((flight) => {
              if (!flight) return flight;
              const updated = { ...flight, stand: result.stand, stand_state: result.stand_state, stand_version: result.version };
              apiFlightRef.current = updated;
              return updated;
            });
            await refreshFlight();
          }}
        />
      )}
      <D2CDMDialog
        isOpen={openDialog === 'D2CDM'}
        onClose={handleCloseDialog}
        position={isArrival ? 'L3' : 'M2'}
        currentTobt={flightData.tobt}
        currentCtot={flightData.ctot}
        onUpdate={arrivalPreview || timingInhibited ? undefined : async (value) => {
          const tobt = value.replace(/Z$/i, '');
          const callsign = apiFlight?.callsign;
          await authorizedFetch(efbPath('/api/efb/tobt'), { method: 'PUT', body: JSON.stringify({ tobt, callsign: trainingSession || profile?.live_mode === false ? devCallsignRef.current : undefined }) });
          if (callsign && trainingSession === trainingSessionRef.current && callsign === apiFlightRef.current?.callsign) {
            setConfirmedTobt({ session: trainingSession, callsign, value: tobt });
          }
          await refreshFlight();
        }}
      />
      <D2ATISDialog
        isOpen={openDialog === 'D2ATIS'}
        onClose={handleCloseDialog}
        position={tilePosition(activeBox ?? 'L2')}
        content={`${apiFlight?.atis?.stale ? 'STALE ' : ''}ATIS for ${flightData.callsign}`}
        atisText={atisTextCurrent}
      />
      <D2PDCDialog
        isOpen={openDialog === 'D2PDC'}
        onClose={handleCloseDialog}
        onConfirm={handlePdcConfirm}
        onUnable={handlePdcUnable}
        position={tilePosition(activeBox ?? 'L2')}
        content={`PDC for ${flightData.callsign}`}
        pdcText={apiFlight?.pdc_clearance_text || ''}
      />

      {actionError && (
        <button type="button" onClick={() => setActionError(null)} className="absolute top-[1%] left-1/2 z-[900] -translate-x-1/2 border-2 border-[#B63F3F] bg-[#000109] px-4 py-2 text-center font-bold text-white">
          {actionError}
        </button>
      )}

      {loadError && (
        <button type="button" onClick={() => setLoadError(null)} className="absolute top-[7%] left-1/2 z-[899] -translate-x-1/2 border-2 border-[#D9C01E] bg-[#000109] px-4 py-2 text-center font-bold text-white">
          {flightStale ? `UPDATE FAILED — DATA MAY BE STALE: ${loadError}` : loadError}
        </button>
      )}

      {!arrivalPreview && apiFlight?.data_errors?.length ? (
        <div role="alert" className="absolute top-[7%] left-1/2 z-[898] -translate-x-1/2 border-2 border-[#D9C01E] bg-[#000109] px-4 py-2 text-center font-bold text-white">
          {apiFlight.data_errors.join(' ')}
        </div>
      ) : null}

      <div className="absolute top-[2.5%] left-0 h-[10%] w-full">
        {showFlightplanElements === false && (
          <a href="https://www.simbrief.com/home/" target="_blank" rel="noreferrer" className="absolute top-[2.5%] left-[2.5%] flex h-[84.16%] w-[30.83%] cursor-pointer items-center justify-center border-0 bg-[#2E343D] text-xs text-white">
            <img src={simbriefFplImg} alt="L1 Picture" className="h-full w-full object-cover" />
          </a>
        )}

        {showFlightplanElements && (
          <button type="button" aria-label="Downloads" onClick={handleDownloadsClick} className="efb-action-bar efb-clickspot absolute top-[2.5%] left-[2.5%] flex h-[60%] w-[30.83%] cursor-pointer items-center justify-center border-0 bg-[#011328] text-center text-[clamp(14px,3.5vh,53px)] font-bold text-white">
            <img src={standArrow} alt="" className="absolute left-[3%] h-[85%] w-[12%] object-contain" />
            DOWNLOADS
          </button>
        )}

        <button type="button" onClick={() => { if (!showFlightplanElements) void (profile === null ? loadProfile() : refreshFlight()); }} className={`absolute top-[2.5%] left-[34.58%] flex h-[60%] w-[30.83%] items-center justify-center bg-[#011328] text-center text-[clamp(24px,5.5vh,106px)] font-bold text-white ${showFlightplanElements ? 'cursor-default' : 'cursor-pointer'}`}>
          {showFlightplanElements ? flightData.callsign : loadState === 'LOADING' ? 'LOADING' : loadState === 'ERROR' ? 'CLICK TO RETRY' : 'CLICK TO REFRESH'}
        </button>

        <div className={`absolute top-[66.66%] left-[34.58%] flex h-[40%] w-[30.83%] items-center justify-center text-center text-[clamp(24px,3.5vh,106px)] font-bold text-white ${getM1HeaderColor()}`}>
          {getM1HeaderText()}
        </div>

        {showFlightplanElements === false && (
          <a href="https://my.vatsim.net/pilots/flightplan" target="_blank" rel="noreferrer" className="absolute top-0 left-[66.66%] flex h-[86.66%] w-[30.83%] cursor-pointer items-center justify-center border-0 bg-white text-xs text-[#999]">
            <img src={fileVatsimFplImg} alt="R1 Picture" className="h-full w-full object-cover" />
          </a>
        )}

        {showFlightplanElements && (
          <a href="https://flightboard.simfixr.com/?icao=EKCH" aria-label="Traffic Board" target="_blank" rel="noreferrer" className="efb-action-bar efb-clickspot absolute top-[2.5%] left-[66.66%] flex h-[60%] w-[30.83%] items-center justify-center border-0 bg-[#011328] text-center text-[clamp(14px,3.5vh,53px)] font-bold text-white">
            TRAFFIC BOARD
            <img src={standArrow} alt="" className="absolute right-[3%] h-[85%] w-[12%] object-contain" />
          </a>
        )}
      </div>

      <div className="contents">
        <div data-efb-tile={tilePosition('L2')} style={tileStyle('L2')} className={`absolute w-[30.83%] border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('L2')} ${showFlightplanElements ? 'cursor-pointer' : 'cursor-default'}`} onClick={() => handleBoxClick('L2')} onMouseEnter={() => setHoveredBox('L2')} onMouseLeave={() => setHoveredBox(null)}>
          <ArrivalTileContent key={isArrival ? 'charts' : 'stand'} title={isArrival ? 'Charts' : 'Stand'} onInfoHoverChange={setInfoHovered} info={isArrival ? arrivalTileInfo.L2 : departureTileInfo.L2}>
          {showFlightplanElements ? (
            isArrival ? (
              <img src={chartsImage} alt="Charts" className="h-[95%] w-[95%] object-contain" />
            ) : (
              <>
                <div role="group" aria-label="Stand" className={`efb-action-bar ${apiFlight?.capabilities.stand_reassignment ? 'efb-clickspot' : ''} absolute top-0 left-0 flex h-1/2 w-full flex-col border-[clamp(5px,0.5vw,18px)] border-[#000109] ${getStandM1Color()}`}>
                  <button type="button" aria-label="Open stand dialog" onClick={(event) => { event.stopPropagation(); handleBoxClick('L2'); }} className="flex h-full w-full cursor-pointer flex-col focus-visible:outline-2 focus-visible:outline-[#43C6E7]">
                    <div className="flex h-[40%] w-full items-center justify-center text-[clamp(14px,3.5vh,88px)] font-normal text-white">{getL2Title()}</div>
                    <div className="flex h-[66%] w-full items-center justify-center text-[clamp(14px,8.5vh,132px)] font-bold text-white">{flightData.stand}</div>
                  </button>
                </div>
                <div className="absolute top-1/2 left-0 flex h-[30%] w-full items-center justify-center bg-[#000109]">
                  <img src={standImage} alt="Stand Image" className="h-[90%] w-[90%] object-contain" />
                </div>
                <div role="status" aria-label="Stand state" className={`efb-action-bar ${apiFlight?.capabilities.stand_reassignment ? 'efb-clickspot' : ''} absolute top-[80%] left-0 flex h-[20%] w-full items-center justify-center border-[clamp(5px,0.5vw,18px)] border-[#000109] px-2 text-center ${standStateTextClass} font-bold text-white ${getStandM3Color()}`}>{getStandM3Text()}</div>
              </>
            )
          ) : (
            <div className="flex h-full w-full items-center justify-center bg-[#000109]">
              <img src={loadingState} alt="Loading" className="h-[60%] w-[60%] object-contain" />
            </div>
          )}
          </ArrivalTileContent>
        </div>

        <div data-efb-tile={tilePosition('M2')} style={tileStyle('M2')} className={`absolute w-[30.83%] cursor-default border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('M2')}`} onMouseEnter={() => setHoveredBox('M2')} onMouseLeave={() => setHoveredBox(null)}>
          <ArrivalTileContent key={isArrival ? 'arrival' : 'departure'} title={isArrival ? 'Arrival information' : 'Departure information'} onInfoHoverChange={setInfoHovered} info={isArrival ? arrivalTileInfo.M2 : departureTileInfo.M2}>
          {/* unchanged */}
          {showFlightplanElements ? (
            <>
              {!isArrival && <>
                {([
                  ['Initial Climb', flightData.initialClimb, 'top-0 left-[2%]', false],
                  ['Departure Freq', flightData.depFrequency, 'top-0 right-[2%]', true],
                  ['CTOT', flightData.ctot, 'top-[66%] left-[2%]', false],
                  ['SID', flightData.sid, 'top-[66%] right-[2%]', true],
                ] as const).map(([label, value, position, right]) => (
                  <div key={label} role="group" aria-label={label === 'CTOT' ? 'Departure CTOT' : label} className={`absolute ${position} h-[34%] w-[40%] text-white`}>
                    <div className={`flex h-[40%] w-full items-center border-b border-[#38536E] text-[clamp(10px,1.7vh,30px)] font-bold tracking-[0.08em] text-[#B8D9F0] ${right ? 'justify-end text-right' : 'justify-start text-left'}`}>
                      {label.toUpperCase()}
                    </div>
                    <div className={`flex h-[60%] w-full items-start gap-[4%] whitespace-nowrap pt-[2%] text-[clamp(14px,3.5vh,53px)] ${right ? 'justify-end' : 'justify-start'}`}>
                      {label === 'Departure Freq' && <img src={Headset} alt="Headset" className="mt-[3%] aspect-square w-[18%] object-contain" />}
                      {value}
                    </div>
                    {label === 'Departure Freq' && value !== 'NIL' && (
                      <div className="absolute top-[78%] right-0 w-full text-right text-[clamp(12px,1.8vh,32px)] leading-snug">
                        Contact <strong className="underline">AUTOMATICALLY</strong><br />after departure
                      </div>
                    )}
                  </div>
                ))}
                <div role="group" aria-label="Departure runway" className="absolute bottom-[2%] left-[40%] flex w-[18%] justify-center text-[clamp(14px,3.5vh,53px)] text-white">
                  {flightData.assignedRunway}
                </div>
              </>}

              <div className="absolute top-0 left-[34%] -translate-y-[4%] flex h-full w-[30%] items-center justify-center">
                <img src={depinfoImage} alt={isArrival ? 'Arrival Info' : 'Departure Info'} className="h-[90%] w-[90%] object-contain" />
              </div>

              {isArrival && (
                <div role="group" aria-label="Published holding" className="absolute top-0 left-[2%] h-[80%] w-[46%] text-white">
                  <div className="absolute top-[2%] left-0 flex h-[14%] w-[86%] items-center justify-start whitespace-nowrap border-b border-[#38536E] text-left text-[clamp(10px,1.7vh,30px)] font-bold tracking-[0.08em] text-[#B8D9F0]">
                    PUBLISHED HOLDING
                  </div>
                  <img src={HOLD} alt="Holding pattern" className={`absolute top-[16%] left-[2%] h-[46%] w-[30%] object-contain ${flightData.publishedHoldingDetail.endsWith('/LEFT') ? '-scale-x-100' : ''}`} />
                  <div className="absolute top-[16%] left-[35%] flex h-[24%] w-[63%] items-center justify-start whitespace-nowrap text-[clamp(14px,3.5vh,53px)] font-bold">
                    {flightData.publishedHoldingFix || 'NOT AVAILABLE'}
                  </div>
                  <div className="absolute top-[38%] left-[35%] flex h-[20%] w-[63%] items-start justify-start whitespace-nowrap text-[clamp(14px,2.5vh,53px)]">
                    {flightData.publishedHoldingDetail || 'DETAILS UNAVAILABLE'}
                  </div>
                </div>
              )}

              {isArrival && (
                <div role="group" aria-label="Expected APP" className="absolute top-[66%] right-[2%] h-[34%] w-[40%] text-right text-white">
                  <div className="absolute top-0 right-0 flex h-[40%] w-[108%] items-center justify-end border-b border-[#38536E] text-[clamp(10px,1.7vh,30px)] font-bold tracking-[0.08em] text-[#B8D9F0]">
                    EXPECTED APP
                  </div>
                  <div className="absolute top-[40%] right-0 flex h-[60%] w-full items-start justify-end whitespace-nowrap pt-[2%] text-[clamp(14px,3.5vh,53px)]">
                    ILS{flightData.arrivalRunway ? ` ${flightData.arrivalRunway}` : ''}
                  </div>
                </div>
              )}

              {isArrival && <div className="absolute top-0 h-[80%] right-[2%] w-[40%]">
                  <div role="group" aria-label="After passing" className="absolute top-[2%] right-0 flex h-[14%] w-[108%] items-center justify-end border-b border-[#38536E] text-right text-[clamp(10px,1.7vh,30px)] font-bold tracking-[0.08em] text-[#B8D9F0]">
                    AFTER PASSING
                  </div>
                  <div className="absolute top-[16%] right-0 flex h-[24%] w-full items-center justify-end gap-[4%] text-[clamp(14px,3.5vh,53px)] text-white">
                    <div className="flex aspect-square w-[20%] items-center justify-center">
                      <img src={RNAVLOGO} alt="Arrival procedure" className="h-full w-full object-contain" />
                    </div>
                    <div>{flightData.terminalFix || 'STAR'}</div>
                  </div>
                  <div className="absolute top-[38%] right-0 flex h-[20%] w-full flex-col items-end justify-start text-right text-[clamp(14px,1.5vh,36px)] text-white">
                    {flightData.arrivalHeading && <div className="text-[clamp(14px,2.5vh,53px)] underline underline-offset-4">Maintain {flightData.arrivalHeading}</div>}
                  </div>
              </div>}

              {isArrival && (
                <div role="group" aria-label="Expected STAR" className="absolute top-[66%] left-[2%] h-[34%] w-[46%] text-left text-white">
                  <div className="absolute top-0 left-0 flex h-[40%] w-[86%] items-center justify-start border-b border-[#38536E] text-[clamp(10px,1.7vh,30px)] font-bold tracking-[0.08em] text-[#B8D9F0]">
                    EXPECTED STAR
                    {apiFlight?.arrival_guidance_expected && <span className="ml-1 text-[clamp(9px,1.1vh,14px)]">(ROUTE / ATIS)</span>}
                  </div>
                  <div className="absolute top-[40%] left-0 flex h-[60%] w-full items-start justify-start whitespace-nowrap pt-[2%] text-[clamp(14px,3.5vh,53px)]">
                    {flightData.star}
                  </div>
                </div>
              )}
            </>
          ) : (
            <div className="flex h-full w-full items-center justify-center bg-[#000109]">
              <img src={loadingPlaceholder} alt="Loading" className="h-[60%] w-[60%] object-contain" />
            </div>
          )}
          </ArrivalTileContent>
        </div>

        <div data-efb-tile={tilePosition('R2')} style={tileStyle('R2')} className={`absolute flex w-[30.83%] items-center justify-center border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('R2')} ${showFlightplanElements ? 'cursor-pointer' : 'cursor-default'}`} onClick={() => handleBoxClick('R2')} onMouseEnter={() => setHoveredBox('R2')} onMouseLeave={() => setHoveredBox(null)}>
          <ArrivalTileContent key={isArrival ? 'arrival-briefing' : 'charts'} title={isArrival ? 'Arrival briefing' : 'Charts'} onInfoHoverChange={setInfoHovered} info={isArrival ? arrivalTileInfo.R2 : departureTileInfo.R2}>
          {showFlightplanElements ? (
            isArrival ? (
              <button type="button" aria-label="Open arrival briefing" onClick={(event) => { event.stopPropagation(); handleBoxClick('R2'); }} className="flex h-full w-full cursor-pointer items-center justify-center bg-[#000109]">
                <img src={APPBRIEF} alt="Arrival briefing" className="h-[95%] w-[95%] object-contain" />
              </button>
            ) : (
              <img src={chartsImage} alt="Charts" className="h-[95%] w-[95%] object-contain" />
            )
          ) : (
            <img src={loadingPlaceholder} alt="Loading" className="h-[60%] w-[60%] object-contain" />
          )}
          </ArrivalTileContent>
        </div>
      </div>

      <div className="contents">
        <div data-efb-tile={tilePosition('L3')} style={tileStyle('L3')} className={`absolute w-[30.83%] ${isArrival && showFlightplanElements ? 'cursor-pointer' : 'cursor-default'} border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('L3')}`} onClick={() => { if (isArrival) handleBoxClick('L3'); }} onMouseEnter={() => setHoveredBox('L3')} onMouseLeave={() => setHoveredBox(null)}>
          <ArrivalTileContent key={isArrival ? 'timing' : 'atis-pdc'} title={isArrival ? 'TOBT / TSAT / CTOT' : 'ATIS / PDC'} onInfoHoverChange={setInfoHovered} info={isArrival ? arrivalTileInfo.L3 : departureTileInfo.L3}>
          {showFlightplanElements ? (
            isArrival ? (
              timingTile
            ) : (
              <>
                <div onClick={handleL3M1Click} className={`efb-action-bar efb-clickspot absolute top-0 left-0 flex h-1/2 w-full cursor-pointer items-center border-[clamp(5px,0.5vw,18px)] border-[#000109] ${getAtisColor()}`}>
                  <div className="w-[66%] pr-[5%] text-right">
                    <div className="-translate-x-[20%] text-[clamp(14px,5.5vh,88px)] font-bold text-white">{getAtisText()}</div>
                  </div>
                  <div className="flex aspect-square w-[20%] items-center justify-center">
                    <img src={ATISPicture} alt="ATIS" className="h-[120%] w-[120%] object-contain" />
                  </div>
                </div>

                {atisWeatherWidgets}
                <div className="absolute top-[80%] left-0 flex h-[20%] w-full items-center justify-center">
                  <button type="button" onClick={handleL3M2Click} disabled={pdcStatus === 'PENDING' || pdcStatus === 'CONFIRMED'} className={`efb-action-bar efb-clickspot flex h-full w-full min-w-0 cursor-pointer items-center justify-center rounded-none border-[clamp(5px,0.5vw,18px)] border-[#000109] px-[5%] [container-type:inline-size] focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-[#43C6E7] disabled:cursor-default ${getClrM2Color()}`}>
                    <span className={`w-full whitespace-nowrap text-center text-[min(3.5vh,53px,var(--pdc-label-size))] font-bold ${pdcStatus === 'RECEIVED' && !blinkOn ? 'text-[#43C6E7]' : 'text-white'}`} style={{ '--pdc-label-size': `${100 / (getClrM2Text().length * 0.75)}cqw` } as CSSProperties}>
                      {getClrM2Text()}
                    </span>
                  </button>
                </div>
              </>
            )
          ) : (
            <div className="flex h-full w-full items-center justify-center bg-[#000109]">
              <img src={loadingPlaceholder} alt="Loading" className="h-[60%] w-[60%] object-contain" />
            </div>
          )}
          </ArrivalTileContent>
        </div>

        <div data-efb-tile={tilePosition('M3')} style={tileStyle('M3')} className={`absolute w-[30.83%] border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('M3')} ${showFlightplanElements ? 'cursor-pointer' : 'cursor-default'}`} onClick={() => handleBoxClick('M3')} onMouseEnter={() => setHoveredBox('M3')} onMouseLeave={() => setHoveredBox(null)}>
          <ArrivalTileContent key={isArrival ? 'atis' : 'timing'} title={isArrival ? 'ATIS' : 'TOBT / TSAT / CTOT'} onInfoHoverChange={setInfoHovered} info={isArrival ? arrivalTileInfo.M3 : departureTileInfo.M3}>
          {showFlightplanElements ? (
            isArrival ? (
              <button type="button" aria-label="Open ATIS" onClick={(event) => { event.stopPropagation(); handleBoxClick('M3'); }} className="efb-atis-control relative block h-full w-full cursor-pointer border-0 border-[#000109] bg-[#000109]">
                <div role="group" aria-label="ATIS identifier" className={`efb-action-bar efb-clickspot absolute top-0 left-0 flex h-1/2 w-full flex-col items-center justify-center border-[clamp(5px,0.5vw,18px)] border-[#000109] ${getAtisColor()}`}>
                  <div className="flex h-[52%] w-full items-center justify-center text-center text-[clamp(14px,5.5vh,88px)] leading-none font-bold text-white">
                    {apiFlight?.atis?.code ? `ATIS ${apiFlight.atis.code}${apiFlight.atis.stale ? ' STALE' : ''}` : getAtisText()}
                  </div>
                  <div className="flex h-[44%] w-full items-center justify-center">
                    <img src={ATISPicture} alt="ATIS icon" className="h-full w-auto max-w-[30%] object-contain" />
                  </div>
                </div>
                {atisWeatherWidgets}
                <div role="note" className="efb-action-bar absolute top-[80%] left-0 flex h-[20%] w-full items-center justify-center border-[clamp(5px,0.5vw,18px)] border-[#000109] bg-[#1A475F] px-2 text-center text-[clamp(12px,2vh,28px)] leading-tight font-normal text-white">
                  At First Contact with APP, Stats A/C Type &amp; ATIS Info
                </div>
              </button>
            ) : (
              timingTile
            )
          ) : (
            <div className="flex h-full w-full items-center justify-center bg-[#000109]">
              <img src={loadingPlaceholder} alt="Loading" className="h-[60%] w-[60%] object-contain" />
            </div>
          )}
          </ArrivalTileContent>
        </div>

        <div data-efb-tile={tilePosition('R3')} style={tileStyle('R3')} className={`absolute flex w-[30.83%] items-center justify-center border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('R3')} ${showFlightplanElements ? 'cursor-pointer' : 'cursor-default'}`} onClick={() => handleBoxClick('R3')} onMouseEnter={() => setHoveredBox('R3')} onMouseLeave={() => setHoveredBox(null)}>
          <ArrivalTileContent key={isArrival ? 'stand' : 'briefing'} title={isArrival ? 'Stand' : 'Departure briefing'} onInfoHoverChange={setInfoHovered} info={isArrival ? arrivalTileInfo.R3 : departureTileInfo.R3}>
          {showFlightplanElements ? (
            isArrival ? (
              <>
                <div role="group" aria-label="Stand" className={`efb-action-bar ${apiFlight?.capabilities.stand_reassignment ? 'efb-clickspot' : ''} absolute top-0 left-0 flex h-1/2 w-full flex-col border-[clamp(5px,0.5vw,18px)] border-[#000109] ${getStandM1Color()}`}>
                  <button type="button" aria-label="Open stand dialog" onClick={(event) => { event.stopPropagation(); handleBoxClick('R3'); }} className="flex h-full w-full cursor-pointer flex-col focus-visible:outline-2 focus-visible:outline-[#43C6E7]">
                    <div className="flex h-[40%] w-full items-center justify-center text-[clamp(14px,3.5vh,88px)] font-normal text-white">{getL2Title()}</div>
                    <div className="flex h-[66%] w-full items-center justify-center text-[clamp(14px,8.5vh,132px)] font-bold text-white">{flightData.stand}</div>
                  </button>
                </div>

                <div className="absolute top-1/2 left-0 flex h-[30%] w-full items-center justify-center bg-[#000109]">
                  <img src={standImage} alt="Stand Image" className="h-[90%] w-[90%] object-contain" />
                </div>

                <div role="status" aria-label="Stand state" className={`efb-action-bar ${apiFlight?.capabilities.stand_reassignment ? 'efb-clickspot' : ''} absolute top-[80%] left-0 flex h-[20%] w-full items-center justify-center border-[clamp(5px,0.5vw,18px)] border-[#000109] text-[clamp(14px,3.5vh,53px)] font-bold text-white ${getStandM3Color()}`}>{getStandM3Text()}</div>
              </>
            ) : (
              <img src={briefingImage} alt="briefingImage" className="h-[95%] w-[95%] object-contain" />
            )
          ) : (
            <img src={loadingPlaceholder} alt="Loading" className="h-[60%] w-[60%] object-contain" />
          )}
          </ArrivalTileContent>
        </div>
      </div>
    </div>
  );
}
