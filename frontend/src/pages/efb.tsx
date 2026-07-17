import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { useAuth0 } from '@auth0/auth0-react';
import { getApiUrl } from '@/lib/api-url';
import simbriefFplImg from '../assets/efb/simbrief-fpl.png';
import fileVatsimFplImg from '../assets/efb/file-vatsim-fpl.png';
import standImage from '../assets/efb/stand-image.png';
import depinfoImage from '../assets/efb/depinfo-image.png';
import chartsImage from '../assets/efb/charts-image.png';
import briefingImage from '../assets/efb/briefing-image.png';
import loadingPlaceholder from '../assets/efb/loading-placeholder.png';
import loadingState from '../assets/efb/loading-state.png';
import Headset from '../assets/efb/Headset.png';
import PDCPicture from '../assets/efb/PDC-Picture.png';
import ATISPicture from '../assets/efb/ATIS-Picture.png';
import HOLD from '../assets/efb/HOLD.png';
import APPBRIEF from '../assets/efb/APPBRIEF.png';
import RNAVLOGO from '../assets/efb/RNAVLOGO.png';
import DOWNLOADS from '../assets/efb/DOWNLOADS.png';
import TRAFFICBOARD from '../assets/efb/TRAFFICBOARD.png';
import D1BRIEFDialog from '../components/efb/dialogs/D1Brief';
import D1ChartDialog from '../components/efb/dialogs/D1Chart';
import D1DownloadsDialog from '../components/efb/dialogs/D1DownloadsDialog';
import D1STANDDialog from '../components/efb/dialogs/D1Stand';
import D2CDMDialog from '../components/efb/dialogs/D2CDMDialog';
import D2ATISDialog from '../components/efb/dialogs/D2ATISDialog';
import D2PDCDialog from '../components/efb/dialogs/D2PDCDialog';

type PilotState = 'OFFLINE_NO_FP' | 'OFFLINE_WITH_FP' | 'DEPARTURE' | 'ARRIVAL';
type PDCStatus = 'NOATIS' | 'NOREQ' | 'PENDING' | 'RECEIVED' | 'CONFIRMED';
type TOBTStatus = 'DEFAULT' | 'CONFIRMED' | 'ACTIVE' | 'EXPIRED';
type TSATStatus = 'DEFAULT' | 'ACTIVE' | 'EXPIRED';
type AtisUiState = 'READY_TO_REQUEST' | 'RECEIVED_ACK_PENDING' | 'ACKED_CURRENT' | 'NEW_AVAILABLE';
type LoadState = 'LOADING' | 'READY' | 'NO_FLIGHT' | 'ERROR';

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
  approachFrequency: string;
  star: string;
  atisBetter: string;
  tobt: string;
  tobtStatus: TOBTStatus;
  tsat: string;
  tsatStatus: TSATStatus;
  typeofapp?: string;
  Terminalfix?: string;
}

interface ApiFlight {
  callsign: string;
  aircraft_type?: string | null;
  origin: string;
  stand?: string | null;
  stand_version?: number | null;
  cleared_altitude?: number | null;
  ctot?: string | null;
  departure_frequency?: string | null;
  runway?: string | null;
  sid?: string | null;
  star?: string | null;
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
  };
}

interface EfbProfile {
  live_mode: boolean;
  online_callsign?: string | null;
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

const unavailableFlightData: FlightDisplayData = {
  callsign: '', stand: 'NIL', initialClimb: 'NIL', ctot: 'NIL', assignedRunway: 'NIL',
  depFrequency: 'NIL', sid: 'NIL', arrivalAirport: 'NIL', arrivalEta: 'NIL',
  arrivalRunway: 'NIL', approachFrequency: 'NIL', star: 'NIL', atisBetter: '',
  tobt: 'NIL', tobtStatus: 'DEFAULT', tsat: 'NIL', tsatStatus: 'DEFAULT',
  typeofapp: 'NIL', Terminalfix: 'NIL',
};

type DialogType = 'D1BRIEF' | 'D1CHART' | 'D1DOWNLOADS' | 'D1STAND' | 'D2CDM' | 'D2ATIS' | 'D2PDC' | null;
type BoxType = 'L2' | 'M2' | 'R2' | 'L3' | 'M3' | 'R3';

export default function EFBPage() {
  const { getAccessTokenSilently } = useAuth0();
  const [apiFlight, setApiFlight] = useState<ApiFlight | null>(null);
  const apiFlightRef = useRef<ApiFlight | null>(null);
  const [profile, setProfile] = useState<EfbProfile | null>(null);
  const [devCallsign, setDevCallsign] = useState(() => window.sessionStorage.getItem(DEV_CALLSIGN_KEY) ?? '');
  const devCallsignRef = useRef(devCallsign);
  const [pilotState, setPilotState] = useState<PilotState>('OFFLINE_NO_FP');
  const [openDialog, setOpenDialog] = useState<DialogType>(null);
  const [activeBox, setActiveBox] = useState<BoxType | null>(null);
  const [hoveredBox, setHoveredBox] = useState<BoxType | null>(null);

  // PDC flow
  const [pdcStatus, setPdcStatus] = useState<PDCStatus>('NOREQ');

  // ATIS flow (shared for departure L3/M1 and arrival R2)
  const [atisUiState, setAtisUiState] = useState<AtisUiState>('READY_TO_REQUEST');
  const [atisTextCurrent, setAtisTextCurrent] = useState<string>('');
  const [atisLetterCurrent, setAtisLetterCurrent] = useState<string>('');
  const [blinkOn, setBlinkOn] = useState(true);
  const [actionError, setActionError] = useState<string | null>(null);
  const [loadState, setLoadState] = useState<LoadState>('LOADING');
  const [loadError, setLoadError] = useState<string | null>(null);
  const [flightStale, setFlightStale] = useState(false);

  const flightData: FlightDisplayData = apiFlight ? {
    ...unavailableFlightData,
    callsign: apiFlight.callsign,
    stand: apiFlight.stand || 'NIL',
    initialClimb: apiFlight.cleared_altitude ? `FL${Math.round(apiFlight.cleared_altitude / 100)}` : 'NIL',
    ctot: displayClock(apiFlight.ctot) || 'NIL',
    assignedRunway: apiFlight.runway || 'NIL',
    depFrequency: apiFlight.departure_frequency?.trim() || 'NIL',
    sid: apiFlight.sid || 'NIL',
    star: apiFlight.star || 'NIL',
    arrivalAirport: apiFlight.destination,
    arrivalRunway: apiFlight.runway || 'NIL',
    atisBetter: apiFlight.atis?.code || '',
    tobt: displayClock(apiFlight.tobt || apiFlight.eobt) || 'NIL',
    tobtStatus: apiFlight.tobt ? 'CONFIRMED' : 'DEFAULT',
    tsat: displayClock(apiFlight.tsat) || 'NIL',
    tsatStatus: apiFlight.tsat ? 'ACTIVE' : 'DEFAULT',
  } : unavailableFlightData;

  const authorizedFetch = useCallback(async (path: string, init?: RequestInit) => {
    const token = await getAccessTokenSilently();
    const response = await fetch(getApiUrl(path), { ...init, headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}`, ...(init?.headers ?? {}) } });
    const body = await response.json().catch(() => null);
    if (!response.ok) throw new ApiRequestError(body?.error || `Request failed (${response.status})`, response.status);
    return body;
  }, [getAccessTokenSilently]);

  const refreshFlight = useCallback(async (selectedCallsign = devCallsignRef.current) => {
    try {
      if (profile?.live_mode === false && !selectedCallsign.trim()) {
        apiFlightRef.current = null;
        setApiFlight(null);
        setPilotState('OFFLINE_NO_FP');
        setLoadState('NO_FLIGHT');
        setLoadError(null);
        setFlightStale(false);
        return null;
      }
      if (apiFlightRef.current === null) setLoadState('LOADING');
      const query = profile?.live_mode === false ? `?callsign=${encodeURIComponent(selectedCallsign.trim().toUpperCase())}` : '';
      const flight = await authorizedFetch(`/api/efb/flight${query}`) as ApiFlight;
      apiFlightRef.current = flight;
      setApiFlight(flight);
      setPilotState(flight.phase === 'ARRIVAL' ? 'ARRIVAL' : 'DEPARTURE');
      setLoadState('READY');
      setLoadError(null);
      setFlightStale(false);
      setPdcStatus(!flight.atis || flight.atis.stale ? 'NOATIS' : flight.pdc_requires_pilot_action ? 'RECEIVED' : flight.pdc_state === 'CONFIRMED' ? 'CONFIRMED' : flight.pdc_state === 'REQUESTED' ? 'PENDING' : 'NOREQ');
      if (flight.atis) {
        setAtisTextCurrent(flight.atis.text.join('\n'));
        setAtisLetterCurrent(flight.atis.code);
        setAtisUiState('ACKED_CURRENT');
      } else {
        setAtisTextCurrent('');
        setAtisLetterCurrent('');
        setAtisUiState('READY_TO_REQUEST');
      }
      return flight as ApiFlight;
    } catch (error) {
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
  }, [authorizedFetch, profile]);

  const loadProfile = useCallback(async () => {
    setLoadState('LOADING');
    setLoadError(null);
    try {
      const value = await authorizedFetch('/api/efb/me') as EfbProfile;
      setProfile(value);
      if (value.online_callsign) {
        devCallsignRef.current = value.online_callsign;
        setDevCallsign(value.online_callsign);
      } else if (!value.live_mode && !devCallsignRef.current.trim()) {
        setLoadState('NO_FLIGHT');
      }
    } catch (error) {
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

  const submitDevCallsign = (event: FormEvent) => {
    event.preventDefault();
    const normalized = devCallsign.trim().toUpperCase();
    devCallsignRef.current = normalized;
    setDevCallsign(normalized);
    window.sessionStorage.setItem(DEV_CALLSIGN_KEY, normalized);
    void refreshFlight(normalized);
  };

useEffect(() => {
  const id = setInterval(() => {
    setBlinkOn((v) => !v);
  }, 500); // 0.5s
  return () => clearInterval(id);
}, []);

  const isArrival = pilotState === 'ARRIVAL';
  const showFlightplanElements = loadState === 'READY' && pilotState !== 'OFFLINE_NO_FP';

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
    R2: 'D2ATIS',
    L3: null,
    M3: null,
    R3: 'D1STAND',
  };

  const getDialogMap = (): Record<BoxType, DialogType> => {
    return isArrival ? arrivalDialogMap : departureDialogMap;
  };

  const handleBoxClick = (boxType: BoxType) => {
    if (!showFlightplanElements) return;

    // R2 in ARRIVAL now handled by ATIS state machine, not generic box click mapping
    if (isArrival && boxType === 'R2') {
      handleArrivalR2AtisClick();
      return;
    }

    const dialogMap = getDialogMap();
    const dialogType = dialogMap[boxType];
    if (dialogType === 'D1STAND' && !apiFlight?.capabilities.stand_reassignment) {
      setActionError('Stand reassignment is currently unavailable');
      return;
    }
    if (dialogType === 'D2CDM' && !apiFlight?.capabilities.tobt_update) {
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

  const openAtisDialog = (box: BoxType) => {
    setOpenDialog('D2ATIS');
    setActiveBox(box);
  };

  const handleAtisPrimaryClick = async (sourceBox: BoxType) => {
    if (!showFlightplanElements) return;
    setActionError(null);
    const refreshedFlight = await refreshFlight();
    if (!refreshedFlight?.atis) {
      setActionError('ATIS is currently unavailable');
      return;
    }
    openAtisDialog(sourceBox);
  };

  const handleL3M1Click = () => {
    if (isArrival) return;
    void handleAtisPrimaryClick('L3');
  };

  const handleArrivalR2AtisClick = () => {
    if (!isArrival) return;
    void handleAtisPrimaryClick('R2');
  };

  const handleL3M2Click = () => {
    if (isArrival || !showFlightplanElements) return;

    if (pdcStatus === 'NOATIS') {
      setActionError('Current ATIS is required before requesting PDC');
      return;
    }
    if (pdcStatus === 'NOREQ' && apiFlight?.pdc_can_submit) {
      setActionError(null);
      setPdcStatus('PENDING');
      void authorizedFetch('/api/pdc/request', { method: 'POST', body: JSON.stringify({ callsign: flightData.callsign, aircraft_type: baseAircraftType(apiFlight?.aircraft_type), atis: flightData.atisBetter, stand: flightData.stand === 'NIL' ? '' : flightData.stand, remarks: '' }) })
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
    await authorizedFetch('/api/pdc/acknowledge', { method: 'POST', body: JSON.stringify({ callsign: flightData.callsign }) });
    setPdcStatus('CONFIRMED');
    await refreshFlight();
  };

  const handlePdcUnable = async () => {
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
    if (pilotState === 'OFFLINE_WITH_FP') return 'bg-[#1A475F]';
    return flightData.stand === 'NIL' ? 'bg-[#2E343D]' : 'bg-[#1A475F]';
  };

  const getStandM3Color = () => {
    if (!showFlightplanElements) return 'bg-[#2E343D]';
    if (pilotState === 'OFFLINE_WITH_FP') return 'bg-[#2E435F]';
    return flightData.stand === 'NIL' ? 'bg-[#2E343D]' : 'bg-[#1A475F]';
  };

  const getStandM3Text = () => {
    if (pilotState === 'OFFLINE_WITH_FP') return 'ASSIGNED';
    return flightData.stand === 'NIL' ? 'UNAVAILABLE' : 'ASSIGNED';
  };

  const getAtisColor = () => {
    if (atisUiState === 'READY_TO_REQUEST') return 'bg-[#1A475F]';
    if (atisUiState === 'RECEIVED_ACK_PENDING') return 'bg-[#43C6E7]';
    if (atisUiState === 'ACKED_CURRENT') return 'bg-[#41826E]';
    return 'bg-[#43C6E7]';
  };

  const getAtisText = () => {
    if (atisUiState === 'READY_TO_REQUEST') return 'REQ ATIS';
    if (atisUiState === 'RECEIVED_ACK_PENDING') return '';
    if (atisUiState === 'ACKED_CURRENT') return `ATIS ${atisLetterCurrent || '-'}${apiFlight?.atis?.stale ? ' STALE' : ''}`;
    return '     NEW ATIS';
  };

  const getClrM2Color = () => {
    if (pdcStatus === 'NOATIS') return 'bg-[#8C3838]';
    if (pdcStatus === 'NOREQ') return apiFlight?.pdc_can_submit ? 'bg-[#1A475F]' : 'bg-[#2E343D]';
    if (pdcStatus === 'PENDING') return 'bg-[#FF9800]';
    if (pdcStatus === 'RECEIVED') return blinkOn ? 'bg-[#43C6E7]' : 'bg-[#000109]';
    return 'bg-[#41826E]';
  };

  const getClrM2Text = () => {
    if (pdcStatus === 'NOATIS') return 'REQ ATIS';
    if (pdcStatus === 'NOREQ') return apiFlight?.pdc_can_submit ? 'REQ PDC' : 'PDC UNAVAILABLE';
    if (pdcStatus === 'PENDING') return 'STANDBY';
    if (pdcStatus === 'RECEIVED') return 'VIEW PDC';
    return 'PDC COMPLETE';
  };

  const getCdmL1Color = () => {
    const status = flightData.tobtStatus;
    if (status === 'DEFAULT') return 'bg-[#000109]';
    if (status === 'CONFIRMED') return 'bg-[#1A475F]';
    if (status === 'ACTIVE') return 'bg-[#41826E]';
    return 'bg-[#B63F3F]';
  };

  const getCdmL1Text = () => {
    if (flightData.tobtStatus === 'DEFAULT') return 'Based on EOBT';
    return 'Confirmed';
  };

  const getCdmR1Color = () => {
    const status = flightData.tsatStatus;
    if (status === 'DEFAULT' || status === 'EXPIRED') return 'bg-transparent';
    return 'bg-[#1A475F]';
  };

  const getHoverClass = (boxType: BoxType) => (
    hoveredBox === boxType
      ? 'z-[3] scale-[1.015] brightness-[1.06] shadow-[0_10px_24px_rgba(0,0,0,0.35)]'
      : 'z-[1] scale-100 brightness-100 shadow-none'
  );

  return (
    <div className="relative h-screen w-screen overflow-hidden bg-[#1D293D]">
      {profile?.live_mode === false && (
        <form onSubmit={submitDevCallsign} className="absolute top-2 left-2 z-[50] flex gap-1.5 bg-black/70 p-1.5">
          <input aria-label="Development callsign" value={devCallsign} onChange={(event) => setDevCallsign(event.target.value.toUpperCase())} placeholder="CALLSIGN" className="w-[130px] border border-[#43C6E7] bg-[#011328] px-2 py-[5px] font-mono font-bold text-white" />
          <button type="submit" className="cursor-pointer border border-[#43C6E7] bg-[#1A475F] px-3 py-[5px] font-bold text-white">LOAD</button>
        </form>
      )}
      <D1BRIEFDialog
        isOpen={openDialog === 'D1BRIEF'}
        onClose={handleCloseDialog}
        stand={flightData.stand}
        sid={flightData.sid}
      />
      <D1ChartDialog
        isOpen={openDialog === 'D1CHART'}
        onClose={handleCloseDialog}
        airport={isArrival ? flightData.arrivalAirport : apiFlight?.origin || 'NIL'}
        runway={isArrival ? flightData.arrivalRunway : flightData.assignedRunway}
        sid={flightData.sid}
        arrival={isArrival}
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
          onRequest={async (requestedStand) => {
            await authorizedFetch('/api/efb/stand', { method: 'PUT', body: JSON.stringify({ stand: requestedStand, version: apiFlight?.stand_version ?? 0, callsign: profile?.live_mode === false ? devCallsign : undefined }) });
            await refreshFlight();
          }}
        />
      )}
      <D2CDMDialog
        isOpen={openDialog === 'D2CDM'}
        onClose={handleCloseDialog}
        currentTobt={flightData.tobt}
        currentCtot={flightData.ctot}
        onUpdate={async (value) => {
          await authorizedFetch('/api/efb/tobt', { method: 'PUT', body: JSON.stringify({ tobt: value.replace(/Z$/i, ''), callsign: profile?.live_mode === false ? devCallsign : undefined }) });
          await refreshFlight();
        }}
      />
      <D2ATISDialog
        isOpen={openDialog === 'D2ATIS'}
        onClose={handleCloseDialog}
        position={activeBox ?? 'L2'}
        content={`${apiFlight?.atis?.stale ? 'STALE ' : ''}ATIS for ${flightData.callsign}`}
        atisText={atisTextCurrent}
      />
      <D2PDCDialog
        isOpen={openDialog === 'D2PDC'}
        onClose={handleCloseDialog}
        onConfirm={handlePdcConfirm}
        onUnable={handlePdcUnable}
        position={activeBox ?? 'L2'}
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

      <div className="absolute top-[2.5%] left-0 h-[10%] w-full">
        {showFlightplanElements === false && (
          <a href="https://www.simbrief.com/home/" target="_blank" rel="noreferrer" className="absolute top-[2.5%] left-[2.5%] flex h-[84.16%] w-[30.83%] cursor-pointer items-center justify-center border-0 bg-[#2E343D] text-xs text-white">
            <img src={simbriefFplImg} alt="L1 Picture" className="h-full w-full object-cover" />
          </a>
        )}

        {showFlightplanElements && (
          <button type="button" aria-label="Downloads" onClick={handleDownloadsClick} className="absolute top-[2.5%] left-[2.5%] flex h-[84.16%] w-[30.83%] cursor-pointer items-center justify-center border-0 bg-white text-xs text-[#999]">
            <img src={DOWNLOADS} alt="Downloads" className="h-full w-full object-cover" />
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
          <div className="absolute top-0 left-[66.66%] flex h-[86.66%] w-[30.83%] items-center justify-center border-0 bg-white text-xs text-[#999]">
            <img src={TRAFFICBOARD} alt="Traffic Board" className="h-full w-full object-cover" />
          </div>
        )}
      </div>

      <div className="absolute top-[15%] left-0 h-[40%] w-full">
        <div className={`absolute top-0 left-[2.5%] h-full w-[30.83%] border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('L2')} ${showFlightplanElements ? 'cursor-pointer' : 'cursor-default'}`} onClick={() => handleBoxClick('L2')} onMouseEnter={() => setHoveredBox('L2')} onMouseLeave={() => setHoveredBox(null)}>
          {showFlightplanElements ? (
            isArrival ? (
              <img src={chartsImage} alt="Charts" className="h-[95%] w-[95%] object-contain" />
            ) : (
              <>
                <div className={`absolute top-0 left-0 flex h-1/2 w-full flex-col border-[clamp(5px,0.5vw,18px)] border-[#000109] ${getStandM1Color()}`}>
                  <div className="flex h-[40%] w-full items-center justify-center text-[clamp(14px,3.5vh,88px)] font-normal text-white">{getL2Title()}</div>
                  <div className="flex h-[66%] w-full items-center justify-center text-[clamp(14px,8.5vh,132px)] font-bold text-white">{flightData.stand}</div>
                </div>
                <div className="absolute top-1/2 left-0 flex h-[30%] w-full items-center justify-center bg-[#000109]">
                  <img src={standImage} alt="Stand Image" className="h-[90%] w-[90%] object-contain" />
                </div>
                <div className={`absolute top-[80%] left-0 flex h-[20%] w-full items-center justify-center border-[clamp(5px,0.5vw,18px)] border-[#000109] text-[clamp(14px,3.5vh,53px)] font-bold text-white ${getStandM3Color()}`}>{getStandM3Text()}</div>
              </>
            )
          ) : (
            <div className="flex h-full w-full items-center justify-center bg-[#000109]">
              <img src={loadingState} alt="Loading" className="h-[60%] w-[60%] object-contain" />
            </div>
          )}
        </div>

        <div className={`absolute top-0 left-[34.58%] h-full w-[30.83%] cursor-default border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('M2')}`} onMouseEnter={() => setHoveredBox('M2')} onMouseLeave={() => setHoveredBox(null)}>
          {/* unchanged */}
          {showFlightplanElements ? (
            <>
              <div className="absolute top-0 left-0 h-full w-[20%]">
                <div className="absolute top-0 left-[15%] flex h-[20%] w-full items-end justify-end pr-[5%] pb-[5%] text-right text-[clamp(14px,3.5vh,53px)] text-white">
                  {isArrival ? flightData.typeofapp : flightData.initialClimb}
                </div>
                <div className="absolute top-[10%] left-[25%] flex h-[20%] w-[48%] flex-col items-start justify-end whitespace-nowrap text-center text-[clamp(14px,1.5vh,27px)] text-white">
                  {isArrival ? (<><div></div><div></div></>) : (<><div>Initial Climb</div><div>Level</div></>)}
                </div>
                <div className="absolute top-[60%] left-[25%] flex h-[20%] w-[48%] flex-col items-start justify-end pt-[2%] pr-[5%] text-right text-[clamp(14px,1.5vh,27px)] text-white">
                  <div>{isArrival ? 'EAT' : 'CTOT'}</div>
                  <div className="text-[clamp(14px,3.5vh,56px)]">{isArrival ? flightData.arrivalEta : flightData.ctot}</div>
                </div>
                <div className="absolute top-[80%] left-[30%] flex h-[20%] w-full items-start justify-end pt-[2%] pr-[5%] text-right text-[clamp(14px,3.5vh,56px)] text-white">
                  {isArrival ? flightData.arrivalRunway : flightData.assignedRunway}
                </div>
              </div>

              <div className="absolute top-0 left-[25%] flex h-full w-[30%] items-center justify-center">
                <img src={depinfoImage} alt={isArrival ? 'Arrival Info' : 'Departure Info'} className="h-[90%] w-[90%] object-contain" />
              </div>

              <div className="absolute top-0 right-0 h-[80%] w-[45%]">
                <div className="absolute top-0 right-[5%] flex h-1/4 w-[95%] items-end justify-start gap-[5%] pb-[2%] pl-[5%] text-[clamp(14px,3.5vh,53px)] text-white">
                  <div className="flex aspect-square w-[20%] items-center justify-center">
                    <img src={isArrival ? RNAVLOGO : Headset} alt={isArrival ? "ILS" : "Headset"} className="h-full w-full object-contain" />
                  </div>
                  <div>{isArrival ? flightData.Terminalfix : flightData.depFrequency === 'NIL' ? null : flightData.depFrequency}</div>
                </div>
                <div className="absolute top-1/4 right-0 flex h-[20%] w-full flex-col items-start justify-start pt-[2%] pl-[5%] text-left text-[clamp(14px,1.5vh,36px)] text-white">
                  {isArrival ? <div className="text-[clamp(14px,2.5vh,53px)]">NIL</div> : flightData.depFrequency !== 'NIL' ? <><div>CONTACT PASSING</div><div><u>1000FT</u> <strong>AUTOMATICALLY</strong></div></> : null}
                </div>
                <div className="absolute top-full right-[5%] flex h-[20%] w-auto -translate-x-[30%] items-start justify-start pt-[2%] text-right text-[clamp(14px,3.5vh,53px)] text-white">
                  {isArrival ? flightData.star : flightData.sid}
                </div>
              </div>
            </>
          ) : (
            <div className="flex h-full w-full items-center justify-center bg-[#000109]">
              <img src={loadingPlaceholder} alt="Loading" className="h-[60%] w-[60%] object-contain" />
            </div>
          )}
        </div>

        {/* R2 ARRIVAL now uses ATIS state machine */}
        <div className={`absolute top-0 left-[66.66%] flex h-full w-[30.83%] items-center justify-center border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('R2')} ${showFlightplanElements ? 'cursor-pointer' : 'cursor-default'}`} onClick={() => handleBoxClick('R2')} onMouseEnter={() => setHoveredBox('R2')} onMouseLeave={() => setHoveredBox(null)}>
          {showFlightplanElements ? (
            isArrival ? (
              <>
                <div className={`absolute top-0 left-0 flex h-[66.66%] w-full items-center justify-center border-x-[2%] border-y-[3%] border-[#000109] pl-[5%] text-center text-[clamp(14px,3.5vh,53px)] text-white ${getAtisColor()}`}>
                  {getAtisText()}
                </div>
                <div className="absolute bottom-0 left-0 flex h-[33.34%] w-full items-center justify-center">
                  <img src={ATISPicture} alt="ATISPicture" className="h-full w-full object-contain" />
                </div>
              </>
            ) : (
              <img src={chartsImage} alt="Charts" className="h-[95%] w-[95%] object-contain" />
            )
          ) : (
            <img src={loadingPlaceholder} alt="Loading" className="h-[60%] w-[60%] object-contain" />
          )}
        </div>
      </div>

      <div className="absolute top-[57.5%] left-0 h-[40%] w-full">
        <div className={`absolute top-0 left-[2.5%] h-full w-[30.83%] cursor-default border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('L3')}`} onMouseEnter={() => setHoveredBox('L3')} onMouseLeave={() => setHoveredBox(null)}>
          {showFlightplanElements ? (
            isArrival ? (
              <div className="relative flex h-full w-full items-center justify-center" aria-label="Arrival briefing coming soon">
                <img src={APPBRIEF} alt="APPBRIEF" className="h-[95%] w-[95%] object-contain opacity-45" />
                <div className="absolute inset-x-[5%] top-1/2 flex -translate-y-1/2 items-center justify-center border-[clamp(5px,0.5vw,18px)] border-[#000109] bg-[#2E343D] py-[5%] text-center text-[clamp(14px,4vh,74px)] font-bold text-white">
                  COMING SOON
                </div>
              </div>
            ) : (
              <>
                <div onClick={handleL3M1Click} className={`absolute top-0 left-0 flex h-1/2 w-full cursor-pointer items-center border-[clamp(5px,0.5vw,18px)] border-[#000109] ${getAtisColor()}`}>
                  <div className="w-[66%] pr-[5%] text-right">
                    <div className="-translate-x-[20%] text-[clamp(14px,5.5vh,88px)] font-bold text-white">{getAtisText()}</div>
                  </div>
                  <div className="flex aspect-square w-[20%] items-center justify-center">
                    <img src={ATISPicture} alt="ATIS" className="h-[120%] w-[120%] object-contain" />
                  </div>
                </div>

                <div onClick={handleL3M2Click} className={`absolute top-1/2 left-0 flex h-1/2 w-full items-center border-[clamp(5px,0.5vw,18px)] border-[#000109] ${pdcStatus === 'PENDING' || pdcStatus === 'CONFIRMED' ? 'cursor-default' : 'cursor-pointer'} ${getClrM2Color()}`}>
                  <div className="flex aspect-square w-[20%] items-center justify-center">
                    <img src={PDCPicture} alt="PDC" className="h-[120%] w-[120%] object-contain" />
                  </div>
                  <div className="w-[66%] pr-[5%] text-right">
                    <div className={`w-full translate-x-[20%] text-center text-[clamp(14px,5.5vh,88px)] font-bold ${pdcStatus === 'RECEIVED' && !blinkOn ? 'text-[#43C6E7]' : 'text-white'}`}>
                      {getClrM2Text()}
                    </div>
                  </div>
                </div>
              </>
            )
          ) : (
            <div className="flex h-full w-full items-center justify-center bg-[#000109]">
              <img src={loadingPlaceholder} alt="Loading" className="h-[60%] w-[60%] object-contain" />
            </div>
          )}
        </div>

        <div className={`absolute top-0 left-[34.58%] h-full w-[30.83%] border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('M3')} ${!showFlightplanElements || isArrival ? 'cursor-default' : 'cursor-pointer'}`} onClick={() => { if (!isArrival) handleBoxClick('M3'); }} onMouseEnter={() => setHoveredBox('M3')} onMouseLeave={() => setHoveredBox(null)}>
          {showFlightplanElements ? (
            isArrival ? (
              <div className="relative h-full w-full border-[clamp(5px,0.5vw,18px)] border-[#000109] text-white">
                <img src={HOLD} alt="Holding pattern" className="h-full w-auto object-contain" />
                <div className="absolute top-0 left-0 flex h-[20%] w-full items-center justify-center text-[clamp(14px,3.5vh,65px)] font-normal">PUBLISHED HOLDING</div>
                <div className="absolute top-1/2 left-1/2 flex h-1/4 w-1/2 items-center justify-center text-[clamp(14px,7.5vh,132px)] font-bold">NIL</div>
                <div className="absolute top-3/4 left-1/2 flex h-[15%] w-1/2 translate-x-[5%] items-center justify-center text-[clamp(14px,3.5vh,65px)] font-normal">NIL</div>
              </div>
            ) : (
              <>
                <div className={`absolute top-0 left-0 h-1/2 w-1/2 border-[clamp(5px,0.5vw,18px)] border-[#000109] ${getCdmL1Color()}`}>
                  <div className="absolute top-[8%] left-0 w-full text-center text-[clamp(14px,3.5vh,74px)] leading-none font-normal text-white">TOBT</div>
                  <div className="absolute top-[38%] left-0 w-full text-center text-[clamp(14px,5.5vh,106px)] leading-none font-bold text-white">{flightData.tobt}</div>
                  <div className="absolute bottom-[8%] left-0 w-full text-center text-[clamp(14px,2.5vh,88px)] leading-none font-normal text-white">{getCdmL1Text()}</div>
                </div>

                <div className={`absolute top-0 left-1/2 h-1/2 w-1/2 border-[clamp(5px,0.5vw,18px)] border-[#000109] ${getCdmR1Color()}`}>
                  <div className="absolute top-[8%] left-0 w-full text-center text-[clamp(14px,3.5vh,74px)] leading-none font-normal text-white">TSAT</div>
                  {flightData.tsatStatus !== 'DEFAULT' && flightData.tsatStatus !== 'EXPIRED' && (
                    <div className="absolute top-[38%] left-0 w-full text-center text-[clamp(14px,5.5vh,106px)] leading-none font-bold text-white">{flightData.tsat}</div>

                  )}
                </div>

                <div className={`absolute top-1/2 left-0 flex h-1/4 w-full items-center justify-center border-[clamp(5px,0.5vw,18px)] border-[#000109] text-[clamp(14px,3.5vh,88px)] font-bold text-white ${apiFlight?.capabilities.tobt_update ? 'bg-[#1A475F]' : 'bg-[#2E343D]'}`}>{apiFlight?.capabilities.tobt_update ? 'CLICK TO UPDATE' : 'UPDATE UNAVAILABLE'}</div>
                <div className="absolute top-3/4 left-0 flex h-1/4 w-full items-center justify-center border-[clamp(5px,0.5vw,18px)] border-[#000109] bg-[#3E5F2E] text-[clamp(14px,3.5vh,88px)] font-bold text-white">PILOT TOBT</div>
              </>
            )
          ) : (
            <div className="flex h-full w-full items-center justify-center bg-[#000109]">
              <img src={loadingPlaceholder} alt="Loading" className="h-[60%] w-[60%] object-contain" />
            </div>
          )}
        </div>

        <div className={`absolute top-0 left-[66.66%] flex h-full w-[30.83%] items-center justify-center border-2 border-[#1D293D] bg-[#000109] transition-[transform,box-shadow,filter] duration-150 ${getHoverClass('R3')} ${showFlightplanElements ? 'cursor-pointer' : 'cursor-default'}`} onClick={() => handleBoxClick('R3')} onMouseEnter={() => setHoveredBox('R3')} onMouseLeave={() => setHoveredBox(null)}>
          {showFlightplanElements ? (
            isArrival ? (
              <>
                <div className={`absolute top-0 left-0 flex h-1/2 w-full flex-col border-[clamp(5px,0.5vw,18px)] border-[#000109] ${getStandM1Color()}`}>
                  <div className="flex h-[40%] w-full items-center justify-center text-[clamp(14px,3.5vh,88px)] font-normal text-white">{getL2Title()}</div>
                  <div className="flex h-[66%] w-full items-center justify-center text-[clamp(14px,8.5vh,132px)] font-bold text-white">{flightData.stand}</div>
                </div>

                <div className="absolute top-1/2 left-0 flex h-[30%] w-full items-center justify-center bg-[#000109]">
                  <img src={standImage} alt="Stand Image" className="h-[90%] w-[90%] object-contain" />
                </div>

                <div className={`absolute top-[80%] left-0 flex h-[20%] w-full items-center justify-center border-[clamp(5px,0.5vw,18px)] border-[#000109] text-[clamp(14px,3.5vh,53px)] font-bold text-white ${getStandM3Color()}`}>{getStandM3Text()}</div>
              </>
            ) : (
              <img src={briefingImage} alt="briefingImage" className="h-[95%] w-[95%] object-contain" />
            )
          ) : (
            <img src={loadingPlaceholder} alt="Loading" className="h-[60%] w-[60%] object-contain" />
          )}
        </div>
      </div>
    </div>
  );
}
