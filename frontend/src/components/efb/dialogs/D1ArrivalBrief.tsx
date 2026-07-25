import { useState } from 'react';

const briefingImages = import.meta.glob<string>('../../../assets/efb/arrival-briefing/*.webp', {
  eager: true,
  import: 'default',
});

const image = (filename: string) => {
  const source = briefingImages[`../../../assets/efb/arrival-briefing/${filename}.webp`];
  if (!source) throw new Error(`Missing arrival briefing asset: ${filename}`);
  return source;
};

type RunwayVariant = '04' | '22';

interface ArrivalBriefPage {
  id: string;
  image: string;
  imageAlt: string;
  title: string;
  description: string[];
}

interface ArrivalBriefProps {
  isOpen: boolean;
  onClose: () => void;
  stand: string;
  star: string;
  runway: string;
  holdingFix: string;
  holdingDetail: string;
  terminalFix: string;
  arrivalHeading: string;
}

interface StarData {
  family: string;
  holdingFix: string;
  holdingCourse: string;
  holdingTurn: string;
  speedFix: string;
  altitudeFix: Record<RunwayVariant, string>;
  intermediateFix: Record<RunwayVariant, string>;
  terminalFix: Record<RunwayVariant, string>;
  downwindHeading: Record<RunwayVariant, string>;
  introVariation: 1 | 2 | 3;
}

const normalize = (value: string) => value.trim().toUpperCase();

const runwayVariant = (runway: string): RunwayVariant => normalize(runway).includes('22') ? '22' : '04';

const STAR_DATA: StarData[] = [
  { family: 'TUDLO', holdingFix: 'LUGAS', holdingCourse: '073', holdingTurn: 'left', speedFix: 'KOR', altitudeFix: { '04': 'CH751', '22': 'CH654' }, intermediateFix: { '04': 'CH727', '22': 'CH626' }, terminalFix: { '04': 'ERPUK', '22': 'ABEGI' }, downwindHeading: { '04': '217', '22': '037' }, introVariation: 1 },
  { family: 'TESPI', holdingFix: 'ROSBI', holdingCourse: '103', holdingTurn: 'left', speedFix: 'TNO', altitudeFix: { '04': 'CH750', '22': 'CH653' }, intermediateFix: { '04': 'CH727', '22': 'CH626' }, terminalFix: { '04': 'ERPUK', '22': 'ABEGI' }, downwindHeading: { '04': '217', '22': '037' }, introVariation: 2 },
  { family: 'ERNOV', holdingFix: 'ERNOV', holdingCourse: '179', holdingTurn: 'left', speedFix: 'ERNOV', altitudeFix: { '04': 'ERNOV', '22': 'ERNOV' }, intermediateFix: { '04': 'CH727', '22': 'CH626' }, terminalFix: { '04': 'ERPUK', '22': 'ABEGI' }, downwindHeading: { '04': '217', '22': '037' }, introVariation: 3 },
  { family: 'TIDVU', holdingFix: 'TIDVU', holdingCourse: '294', holdingTurn: 'right', speedFix: 'ESJAH', altitudeFix: { '04': 'ESJAH', '22': 'ESJAH' }, intermediateFix: { '04': 'CH724', '22': 'CH625' }, terminalFix: { '04': 'DOPEM', '22': 'ADOVI' }, downwindHeading: { '04': '217', '22': '037' }, introVariation: 1 },
  { family: 'MONAK', holdingFix: 'OLPIB', holdingCourse: '030', holdingTurn: 'right', speedFix: 'NEKSO', altitudeFix: { '04': 'NEKSO', '22': 'CH643' }, intermediateFix: { '04': 'CH724', '22': 'CH625' }, terminalFix: { '04': 'DOPEM', '22': 'ADOVI' }, downwindHeading: { '04': '217', '22': '037' }, introVariation: 2 },
];

const KILO_STANDS = new Set(['A4', 'A6', 'A8', 'A17', 'A18', 'A19', 'A20', 'A21', 'A22', 'A23', 'A25', 'A26', 'A27', 'A28', 'A30', 'A31', 'A32', 'A33', 'A34']);
const ALPHA_STANDS = new Set(['A7', 'A9', 'A11', 'A12', 'A14', 'A15', 'B4', 'B6', 'B8', 'B10', 'B15', 'B17', 'B19']);

const starData = (star: string): StarData => STAR_DATA.find(({ family }) => normalize(star).startsWith(family)) ?? STAR_DATA[0];

const standGroup = (stand: string) => {
  const normalized = normalize(stand);
  if (KILO_STANDS.has(normalized)) return 'kilo';
  if (ALPHA_STANDS.has(normalized)) return 'alpha';
  return 'bravo';
};

const taxiInBriefing = (runway: RunwayVariant, stand: string): Pick<ArrivalBriefPage, 'image' | 'imageAlt' | 'description'> => {
  const group = standGroup(stand);
  if (runway === '22' && group === 'kilo') return {
    image: image('taxiin-22kilo'), imageAlt: 'Taxi-in guidance for Kilo stands on runway 22L',
    description: ['Taxi via taxiway C and then via runway 30, vacating at K3 or K2. Hold short of taxiway K or Z and do not continue without an instruction.', 'Pay close attention to “Taxi via runway 30 and K3”: it authorises entering and taxiing down runway 30 before vacating at K3. Apron will continue the taxi when traffic permits.'],
  };
  if (runway === '22') return {
    image: image('taxiin-22bravo'), imageAlt: 'Taxi-in guidance for Bravo stands on runway 22L',
    description: ['Taxi straight ahead on taxiway B. You can expect an instruction to cross runway 30, but hold short of taxiway Z.', 'Do not taxi further without instruction. You will be transferred to Kastrup Apron for the remainder of the taxi when traffic permits.'],
  };
  if (group === 'kilo') return {
    image: image('taxiin-04kilo'), imageAlt: 'Taxi-in guidance for Kilo stands on runway 04L',
    description: ['Taxi via taxiway C and then via runway 30, vacating at K3 or K2. Hold short of taxiway K or Z and do not continue without an instruction.', '“Taxi via runway 30 and K3” authorises entering and taxiing down runway 30 before vacating at K3. Apron will continue the taxi when traffic permits.'],
  };
  if (group === 'alpha') return {
    image: image('taxiin-04alpha'), imageAlt: 'Taxi-in guidance for Alpha stands on runway 04L',
    description: ['Expect to cross runway 30 via taxiway A. Hold short of taxiway Z and do not taxi further without instruction.', 'Kastrup Apron will continue the taxi when traffic permits. Be vigilant if Apron assigns a turn onto taxiway Z (first) or taxiway Y (second).'],
  };
  return {
    image: image('taxiin-04foxtrot'), imageAlt: 'Taxi-in guidance for Foxtrot stands on runway 04L',
    description: ['After crossing runway 30, be especially vigilant: “Taxi on A, cross runway 30 via F” requires a slight right turn from taxiway A onto F.', 'Hold short immediately before taxiway Z. This routing is always expected for the Foxtrot stands.'],
  };
};

const briefingPages = ({ stand, star, runway, holdingFix, holdingDetail, terminalFix, arrivalHeading }: Omit<ArrivalBriefProps, 'isOpen' | 'onClose'>): ArrivalBriefPage[] => {
  const variant = runwayVariant(runway);
  const data = starData(star);
  const finalRunway = variant === '22' ? '22L' : '04L';
  const holding = holdingFix || data.holdingFix;
  const [course = data.holdingCourse, turn = data.holdingTurn.toUpperCase()] = holdingDetail.split('/');
  const transitionFix = terminalFix || data.terminalFix[variant];
  const heading = arrivalHeading.replace(/^HDG/, '') || data.downwindHeading[variant];
  const taxi = taxiInBriefing(variant, stand);
  const completeVariation = standGroup(stand) === 'kilo' ? 2 : standGroup(stand) === 'alpha' ? 4 : variant === '04' ? 3 : 1;

  return [
    {
      id: 'introduction', image: image(`intro-variation-${data.introVariation}`), imageAlt: 'Arrival briefing introduction', title: 'Welcome to Copenhagen',
      description: ['Flying into EKCH, especially during peak periods, can be challenging. This briefing covers published holdings, STAR restrictions, final vectors, ILS speeds, expeditious runway vacation, taxi routing, and arriving on stand.', 'Pay close attention and use the arrows to continue.'],
    },
    {
      id: 'star-holding', image: image(`hold-${data.family.toLowerCase()}-${variant}`), imageAlt: `${normalize(star) || data.family} holding guidance for runway ${finalRunway}`, title: `STAR and holding: ${normalize(star) || data.family}`,
      description: [`Your STAR is predominantly for runway ${finalRunway}, but the final runway is only confirmed at the latest on downwind and you may be assigned the parallel. Be ready for a late change.`, `The holding for ${normalize(star) || data.family} is ${holding}. The inbound course is ${course}° with ${turn.toLowerCase()} turns. Check your FMC/MCDU has the correct hold programmed.`, 'Once released from holding, if required, continue on the STAR and expect handover to Copenhagen Approach.'],
    },
    {
      id: 'star-restrictions', image: image(`arr-${data.family.toLowerCase()}-${variant}`), imageAlt: `${normalize(star) || data.family} restriction guidance for runway ${finalRunway}`, title: 'STAR restrictions',
      description: [`Respect all published STAR restrictions: 250 kt or less after ${data.speedFix}; ${data.altitudeFix[variant]} or below at ${data.altitudeFix[variant]}; 5,000 ft or below at ${data.intermediateFix[variant]}; and 4,000 ft or below and 220 kt at ${transitionFix}.`, `Important: never turn inbound after ${transitionFix}. Continue on heading ${heading} until ATC provides the inbound vector. Do not report established on the heading or ask what to do; fly the published heading and ATC will attend to you.`],
    },
    {
      id: 'ils', image: image('ontheils'), imageAlt: 'ILS speed guidance', title: 'On the ILS',
      description: ['Be ready for an early turn. EKCH often uses short finals from 5–6 NM, so do not arrive high; the STAR restrictions are designed to help you achieve the profile.', 'Adhere to speed restrictions: normally 180 kt until 6 NM final and 160 kt until 4 NM final. If a restriction has not been cancelled, reduce to 180 kt by 10 NM final and 160 kt after 6 NM final.'],
    },
    {
      id: 'landing', image: image(`vacate-${finalRunway.toLowerCase()}`), imageAlt: `Runway vacation guidance for ${finalRunway}`, title: `Landing and runway vacation: ${finalRunway}`,
      description: variant === '22'
        ? ['Vacate the runway expeditiously. Short-haul aircraft should aim for B4; B5 is excellent when performance permits. For a Boeing 737 use at least autobrake 3; for an Airbus A320 use autobrake MEDIUM.', 'Long-haul aircraft should aim for B4 where possible; spacing is provided to facilitate B3. After vacating, do not call: continue on taxiway A and hold short of runway 30. Call only if ATC has not returned by that point.']
        : ['Vacate the runway expeditiously. Short-haul aircraft should aim for A7, using A6 if unable to vacate earlier.', 'Do not vacate to the right unless specifically instructed; this can put you head-on with departures. After vacating, do not call: continue on taxiway A and hold short of runway 30. Call only if ATC has not returned by that point.'],
    },
    {
      id: 'taxi-in', image: taxi.image, imageAlt: taxi.imageAlt, title: `Taxi to stand ${normalize(stand) || 'unassigned'}`, description: taxi.description,
    },
    {
      id: 'complete', image: image(`complete-variation-${completeVariation}`), imageAlt: 'Arrival briefing complete', title: 'On stand',
      description: ['Follow ATC instructions. These routes are standard guidance, but you may be given a different routing. Keep the apron taxi charts available and ask for clarification if in doubt.', 'Thanks for flying into Copenhagen Airport. We hope to see you again soon.'],
    },
  ];
};

function D1ArrivalBriefContent({ isOpen, onClose, ...briefing }: ArrivalBriefProps) {
  const pages = briefingPages(briefing);
  const [currentPage, setCurrentPage] = useState(0);

  if (!isOpen) return null;

  const page = pages[currentPage];
  const handlePrevious = () => setCurrentPage((previous) => (previous === 0 ? pages.length - 1 : previous - 1));
  const handleNext = () => setCurrentPage((previous) => (previous === pages.length - 1 ? 0 : previous + 1));

  return (
    <div className="fixed inset-0 z-[1000] flex items-center justify-center bg-black/70" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label="Arrival briefing" className="relative flex aspect-[3/2] max-h-[85vh] w-[95%] overflow-hidden rounded-lg border-2 border-[#011328] bg-[#000109]" onClick={(event) => event.stopPropagation()}>
        <div className="flex h-full w-[80%] items-center justify-center border-r-2 border-[#000109] bg-[#000109] p-[10px] pb-20">
          <img src={page.image} alt={page.imageAlt} className="box-border h-full w-full border-2 border-[#011328] object-contain" />
        </div>
        <div className="flex h-full w-[20%] flex-col bg-[#000109] pb-20">
          <div className="flex-1 overflow-auto border-b-[10px] border-[#000109] p-5 text-[#dfebeb]">
            <h2 className="mt-0 mb-[15px] text-[clamp(16px,2.5vh,24px)] font-bold">{page.title}</h2>
            {page.description.map((paragraph) => <p key={paragraph} className="mb-4 text-[clamp(12px,1.5vh,16px)] leading-[1.6] text-[#E0E0E0] last:mb-0">{paragraph}</p>)}
          </div>
          <button type="button" className="box-border flex min-h-20 cursor-pointer items-center justify-center border-[25px] border-[#000109] bg-[#dfebeb]" onClick={onClose}><span className="text-[clamp(14px,5vh,20px)] font-bold text-black">CLICK TO CLOSE</span></button>
        </div>
        <div className="absolute right-0 bottom-0 left-0 box-border flex h-20 items-center justify-center gap-5 border-t-[10px] border-[#1D293D] bg-[#000109] p-[15px]">
          <button type="button" aria-label="Previous briefing page" onClick={handlePrevious} className="flex h-[50px] w-[50px] cursor-pointer items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-[#dfebeb] text-[28px] font-bold text-black">←</button>
          <div className="flex gap-3" aria-label={`Briefing page ${currentPage + 1} of ${pages.length}`}>
            {pages.map((briefingPage, index) => <button key={briefingPage.id} type="button" aria-label={`Go to ${briefingPage.title}`} onClick={() => setCurrentPage(index)} className={`h-4 w-4 cursor-pointer rounded-full border-2 border-[#dfebeb] transition-all duration-300 ${currentPage === index ? 'bg-[#dfebeb]' : 'bg-[#666666]'}`} />)}
          </div>
          <button type="button" aria-label="Next briefing page" onClick={handleNext} className="flex h-[50px] w-[50px] cursor-pointer items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-[#dfebeb] text-[28px] font-bold text-black">→</button>
        </div>
      </div>
    </div>
  );
}

export default function D1ArrivalBrief(props: ArrivalBriefProps) {
  return <D1ArrivalBriefContent key={`${props.stand}:${props.star}:${props.runway}`} {...props} />;
}
