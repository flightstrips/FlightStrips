import { useMemo, useState } from 'react';

const briefingImages = import.meta.glob<string>('../../../assets/efb/departure-briefing/*.webp', {
  eager: true,
  import: 'default',
});

const image = (filename: string) => {
  const source = briefingImages[`../../../assets/efb/departure-briefing/${filename}.webp`];
  if (!source) throw new Error(`Missing departure briefing asset: ${filename}`);
  return source;
};

type RunwayVariant = '04R' | '22R';

interface D1BriefPage {
  id: string;
  image: string;
  imageAlt: string;
  title: string;
  description: string[];
}

interface StandBriefingGroup {
  stands: string[];
  image: string;
  description: string[];
}

interface D1BriefProps {
  isOpen: boolean;
  onClose: () => void;
  stand: string;
  sid: string;
  runway: string;
}

const normalize = (value: string) => value.trim().toUpperCase();

const runwayVariant = (runway: string): RunwayVariant | null => {
  const normalized = normalize(runway);
  if (normalized.includes('04R')) return '04R';
  if (normalized.includes('22R')) return '22R';
  return null;
};

const STAND_BRIEFINGS: StandBriefingGroup[] = [
  {
    stands: ['A31', 'A32', 'A33', 'A34'],
    image: 'A31-A34',
    description: [
      'Confirm the release point with ATC before pushback. The supplied written material says Z1, while the supplied diagram labels the standard release point as Z2; J1 and Z4 may be used as backups.',
      'Do not start an engine until aligned with the taxiway.',
    ],
  },
  {
    stands: ['A25', 'A26', 'A27', 'A28', 'A30'],
    image: 'A25-A30',
    description: [
      'From A25, expect an east-facing pushback; J2 may be used. From A26-A30, expect J2 facing east. Facing west is rare.',
    ],
  },
  {
    stands: ['A4', 'A6', 'A8', 'A18', 'A19', 'A20', 'A21'],
    image: 'A17-A23',
    description: [
      'Expect J4 release points. A50 and Z5 may be used when traffic is busy. For heavy aircraft at A19, expect L2 or L3.',
    ],
  },
  {
    stands: ['A22', 'A23'],
    image: 'A17-A23',
    description: ['Expect pushback to J3 or a pull-forward to J4. A50 and Z5 are not normally used.'],
  },
  {
    stands: ['A12', 'A14', 'A15', 'A17'],
    image: 'A12-A17',
    description: [
      'A12: expect Y1; Z6 and Y0 are also used. A14-A15: expect Y0 or a push/pull to Y1. A17: expect Y0 facing west; Z5 and A50 are also common.',
      'Z5 can be used from these stands, but is not normally used.',
    ],
  },
  {
    stands: ['A7', 'A9', 'A11', 'B4', 'B6', 'B8'],
    image: 'A7-B10',
    description: ['Expect Y1 or Z6. Y2 may be used when traffic demands; Z7 is used only for runway 12 operations. A push/pull to M1 may occasionally be assigned.'],
  },
  {
    stands: ['B10'],
    image: 'A7-B10',
    description: ['Heavy aircraft: expect Z6. Otherwise, expect Y1 or Z6; Y2 may be used when traffic demands, Z7 is used only for runway 12 operations, and M1 may occasionally be assigned.'],
  },
  {
    stands: ['B15', 'B19'],
    image: 'B15=B19',
    description: ['Expect P2 or Y3. Y2 may be used when traffic demands.'],
  },
  {
    stands: ['B17'],
    image: 'B15=B19',
    description: ['Heavy aircraft: expect Z8. Otherwise, expect P2 or Y3; Y2 may be used when traffic demands.'],
  },
  {
    stands: ['B7', 'B9'],
    image: 'B7-B9',
    description: ['Expect a push/pull to P2. P1 may be used if P2 is occupied.'],
  },
  {
    stands: ['C28', 'C30', 'C32'],
    image: 'C30-C32',
    description: ['Expect a push/pull to Q2. Q1 may be used if Q2 is occupied.'],
  },
  {
    stands: ['C34', 'C36'],
    image: 'C34-C36',
    description: ['Expect a push/pull to P2. P1 may be used if P2 is occupied. Y4 or Z9 may rarely be used for traffic management.'],
  },
  {
    stands: ['D1', 'D2'],
    image: 'D1-D4',
    description: ['Standard pushback is R1 facing north. S1 facing north is an alternative. A long pull-forward to S2 may be used; pushback to S3 is rare.'],
  },
  {
    stands: ['D3', 'D4', 'E20'],
    image: 'D1-D4',
    description: ['Standard pushback is S1 facing north. R1 facing north is an alternative. A long pull-forward to S2 may be used; pushback to S3 is rare.'],
  },
  {
    stands: ['C29'],
    image: 'C29-C35',
    description: ['Heavy aircraft: expect a push/pull forward to R2 because the aircraft is too large to taxi past D1-D4. Otherwise, expect R1 facing north or a push/pull forward to R2. R1 facing south is not permitted because of jet blast.'],
  },
  {
    stands: ['C33', 'C35'],
    image: 'C29-C35',
    description: ['Expect a push/pull forward to R2. R1 facing south is not permitted because of jet blast.'],
  },
  {
    stands: ['C37'],
    image: 'C37-C39',
    description: ['Heavy aircraft: expect R2; a pull-forward to R4 may be used when traffic demands. R3 and W2 may occasionally be required; W1 is not permitted for this aircraft size. Otherwise, expect R2; R4, W1, R3, or W2 may also be assigned.'],
  },
  {
    stands: ['C39'],
    image: 'C37-C39',
    description: ['Heavy aircraft: expect pushback to B for 22L or V3/V4 for 04R. Otherwise, a push/pull to R4 is most common; R2 and other routes may occasionally be assigned.'],
  },
  {
    stands: ['E22'],
    image: 'E20-E26',
    description: ['Expect a push/pull forward to S2. For non-heavy aircraft, a push/pull to S1 facing north is also possible. S1 facing south is not permitted because of jet blast.'],
  },
  {
    stands: ['E24', 'E36'],
    image: 'E20-E26',
    description: ['Expect a push/pull forward to S2. A push/pull to S1 facing north is possible. S1 facing south is not permitted because of jet blast.'],
  },
  {
    stands: ['F1', 'F4'],
    image: 'F1-F4',
    description: ['Expect an east- or west-facing pushback. W2 and W3 release points are not mandatory unless ATC assigns them for inbound-stand traffic management. S2 or S3 may occasionally be used; F89 is rare.'],
  },
  {
    stands: ['F5', 'F7', 'F8', 'F9'],
    image: 'F5-F9',
    description: ['Expect an east- or west-facing pushback. W2 and W3 release points are not mandatory unless ATC assigns them for inbound-stand traffic management. S2 or S3 may occasionally be used; F89 is rare.'],
  },
  {
    stands: ['E25', 'E29', 'E31', 'E35'],
    image: 'E25-E35',
    description: ['Expect a push/pull to U2. U1 is used only when absolutely necessary for traffic management.'],
  },
  {
    stands: ['E27', 'E33'],
    image: 'E25-E35',
    description: ['Expect a push/pull to U2. For heavy aircraft, U1 is not permitted; otherwise it is used only when absolutely necessary for traffic management.'],
  },
  {
    stands: ['E70', 'E71', 'E72', 'E73', 'E74', 'E75'],
    image: 'E70-E75',
    description: ['Expect pushback to T2, T3, or T4, depending on the stand and traffic requirements.'],
  },
  {
    stands: ['E76', 'E77', 'E78'],
    image: 'E76-E78',
    description: ['Expect pushback to T4 or T5, depending on the stand and traffic requirements.'],
  },
  {
    stands: ['E82', 'E83', 'E84', 'E85', 'E86', 'E87', 'E88', 'E89', 'E90'],
    image: 'E82-E90',
    description: ['Expect pushback to T4 or T5, depending on the stand and traffic requirements.'],
  },
];

const standBriefing = (stand: string): StandBriefingGroup => {
  const matchingBriefing = STAND_BRIEFINGS.find((briefing) => briefing.stands.includes(normalize(stand)));
  return matchingBriefing ?? {
    stands: [],
    image: 'PUSHBACK READY',
    description: [
      `No stand-specific pushback diagram is available for ${normalize(stand) || 'this stand'}. Follow the pushback clearance and any assigned release point.`,
    ],
  };
};

const sidBriefing = (sid: string, runway: RunwayVariant | null): Pick<D1BriefPage, 'image' | 'imageAlt' | 'description'> => {
  const normalizedSid = normalize(sid);
  const sidImageSuffix = runway === '22R' ? '22' : '04';
  const imageFor = (prefix: string) => image(`${prefix}-${sidImageSuffix}`);

  if (['NEXEN', 'LANGO', 'KOPEX'].some((name) => normalizedSid.startsWith(name))) {
    return {
      image: imageFor('NEX-KOP-LAN'),
      imageAlt: `${runway ?? 'assigned runway'} SID chart for ${normalizedSid}`,
      description: [
        'Automatically contact Kastrup Departure on 124.980 when passing 1,000 ft. Follow the SID, but expect a potential direct from departure. Expect further climb to FL190 when traffic permits.',
        normalizedSid.startsWith('KOPEX') ? 'KOPEX is for propeller aircraft only.' : 'NEXEN and LANGO are for jet aircraft only.',
      ],
    };
  }

  if (['ODDON', 'GOLGA', 'VEDAR'].some((name) => normalizedSid.startsWith(name))) {
    return {
      image: imageFor('VED-GOL-ODD'),
      imageAlt: `${runway ?? 'assigned runway'} SID chart for ${normalizedSid}`,
      description: ['Automatically contact Kastrup Departure on 120.255 when passing 1,000 ft. Follow the SID, but expect a potential direct from departure. Expect further climb to FL190 when traffic permits.'],
    };
  }

  if (normalizedSid.startsWith('KEMAX')) {
    return {
      image: imageFor('KEM'),
      imageAlt: `${runway ?? 'assigned runway'} SID chart for ${normalizedSid}`,
      description: ['Automatically contact Kastrup Departure on 124.980 when passing 1,000 ft. Follow the SID, but expect a potential direct from departure. Expect further climb to FL190 when traffic permits.'],
    };
  }

  if (['SIMEG', 'SALLO'].some((name) => normalizedSid.startsWith(name))) {
    return {
      image: imageFor('SIM-SAL'),
      imageAlt: `${runway ?? 'assigned runway'} SID chart for ${normalizedSid}`,
      description: ['Automatically contact Kastrup Departure on 124.980 when passing 1,000 ft. Follow the SID, but expect a potential direct from departure. Expect further climb to FL190 when traffic permits.'],
    };
  }

  if (normalizedSid.startsWith('BETUD')) {
    return {
      image: imageFor('NEX-KOP-LAN'),
      imageAlt: `${runway ?? 'assigned runway'} SID chart for ${normalizedSid}`,
      description: ['Automatically contact Kastrup Departure on 124.980 when passing 1,000 ft. BETUD is only for aircraft that are not allowed through Swedish airspace. If assigned, you will most likely be re-cleared to another SID.'],
    };
  }

  return {
    image: image('SIDNEXT'),
    imageAlt: 'SID guidance',
    description: [`No SID-specific briefing is available for ${normalizedSid || 'the assigned SID'}. Review the current chart and comply with ATC instructions.`],
  };
};

const briefingPages = (stand: string, sid: string, runway: string): D1BriefPage[] => {
  const standPage = standBriefing(stand);
  const runwayForBriefing = runwayVariant(runway);
  const runwayLabel = runwayForBriefing ?? 'assigned runway';
  const sidPage = sidBriefing(sid, runwayForBriefing);

  const taxiInitial = runwayForBriefing === '22R'
    ? {
        image: image('TAXIINIT22R'),
        description: ['Via the standard taxi routes, expect to hold short of runway 30 via taxiway A or F. Taxiway D may be used when traffic demands.', 'Some stands permit departure taxi via K2/K3 and runway 12. Be especially careful when joining taxiway F; it is a hotspot for wrong taxi. Clarify the instruction if in doubt.'],
      }
    : {
        image: image('TAXIINIT04R'),
        description: ['Via the standard taxi routes, expect to hold short of runway 30 via taxiway D or B, depending on the stand.', 'Some stands permit departure taxi via K2/K3 and runway 12. Be especially careful when joining taxiway F; it is a hotspot for wrong taxi. Clarify the instruction if in doubt.'],
      };
  const taxiNext = runwayForBriefing === '22R'
    ? {
        image: image('TAXINEXT22R'),
        description: ['TWR will clear you to cross runway 30 and assign holding point B1-B3 as required for traffic sequencing. Medium and smaller aircraft are expected to be able to depart from B3. Heavy aircraft may depart from B1, or on request.', 'When cleared for take-off, do not linger. During busy periods, sharp separation is required, so begin the take-off roll without delay.'],
      }
    : {
        image: image('TAXINEXT04R'),
        description: ['TWR will clear you to cross runway 30 and assign holding point A1-A4 as required for traffic sequencing. Plan take-off performance for every holding point.', 'Be ready for departure when reaching the holding point. Advise ATC in advance if you are not ready so that an alternative holding point can be assigned. When cleared for take-off, do not linger.'],
      };

  return [
    {
      id: 'pushback-ready',
      image: image('PUSHBACK READY'),
      imageAlt: 'Pushback readiness',
      title: 'Pushback ready',
      description: ['Confirm the stand position, SID, and runway in use before continuing.', 'Your tug must be connected, GSX ready for pushback, and TOBT within +/- 5 minutes of the current time. Read the Pushback Manual and be ready for any assigned release point.'],
    },
    {
      id: 'pushback-next',
      image: image(standPage.image),
      imageAlt: `Pushback guidance for stand ${normalize(stand) || 'unavailable'}`,
      title: `Pushback from ${normalize(stand) || 'your stand'}`,
      description: standPage.description,
    },
    {
      id: 'taxi-initial',
      image: taxiInitial.image,
      imageAlt: `Initial taxi guidance for runway ${runwayLabel}`,
      title: `Initial taxi: ${runwayLabel}`,
      description: taxiInitial.description,
    },
    {
      id: 'taxi-next',
      image: taxiNext.image,
      imageAlt: `Taxi next guidance for runway ${runwayLabel}`,
      title: `Taxi with EKCH_TWR: ${runwayLabel}`,
      description: taxiNext.description,
    },
    {
      id: 'automatic-handover',
      image: image('HANDOVER'),
      imageAlt: 'Automatic handover',
      title: 'Automatic handover',
      description: ['Copenhagen has two departure frequencies. Automatically contact the applicable departure frequency when passing 1,000 ft. EKCH_TWR will not provide the frequency; a “Goodbye” is the final contact.', 'Check the correct frequency on the chart, tune COM2 to it, set COM1 to standby before take-off, and switch and check in immediately when passing 1,000 ft. Copenhagen/Kastrup uses NADP2, so begin acceleration no later than 1,500 ft.'],
    },
    {
      id: 'sid-initial',
      image: sidPage.image,
      imageAlt: sidPage.imageAlt,
      title: `SID: ${normalize(sid) || 'unavailable'}`,
      description: sidPage.description,
    },
    {
      id: 'sid-next',
      image: image('SIDNEXT'),
      imageAlt: 'SID next',
      title: 'SID next',
      description: ['Initial climb is FL70. Maintain 250 kt or less below FL70 unless ATC instructs “Free Speed” or “High Speed Approved”.', 'The transition altitude on this SID is 5,000 ft.'],
    },
    {
      id: 'complete',
      image: image('COMPLETE'),
      imageAlt: 'Departure briefing complete',
      title: 'Departure briefing complete',
      description: ['We wish you a pleasant flight.'],
    },
  ];
};

function D1BriefContent({ isOpen, onClose, stand, sid, runway }: D1BriefProps) {
  const pages = useMemo(() => briefingPages(stand, sid, runway), [stand, sid, runway]);
  const [currentPage, setCurrentPage] = useState(0);

  if (!isOpen) return null;

  const handlePrevious = () => {
    setCurrentPage((previous) => (previous === 0 ? pages.length - 1 : previous - 1));
  };

  const handleNext = () => {
    setCurrentPage((previous) => (previous === pages.length - 1 ? 0 : previous + 1));
  };

  const page = pages[currentPage];

  return (
    <div className="fixed inset-0 z-[1000] flex items-center justify-center bg-black/70" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Departure briefing"
        className="relative flex aspect-[3/2] max-h-[85vh] w-[95%] overflow-hidden rounded-lg border-2 border-[#011328] bg-[#000109]"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex h-full w-[80%] items-center justify-center border-r-2 border-[#000109] bg-[#000109] p-[10px] pb-20">
          <img src={page.image} alt={page.imageAlt} className="box-border h-full w-full border-2 border-[#011328] object-contain" />
        </div>

        <div className="flex h-full w-[20%] flex-col bg-[#000109] pb-20">
          <div className="flex-1 overflow-auto border-b-[10px] border-[#000109] p-5 text-[#dfebeb]">
            <h2 className="mt-0 mb-[15px] text-[clamp(16px,2.5vh,24px)] font-bold">{page.title}</h2>
            {page.description.map((paragraph) => (
              <p key={paragraph} className="mb-4 text-[clamp(12px,1.5vh,16px)] leading-[1.6] text-[#E0E0E0] last:mb-0">{paragraph}</p>
            ))}
          </div>

          <button type="button" className="box-border flex min-h-20 cursor-pointer items-center justify-center border-[25px] border-[#000109] bg-[#dfebeb]" onClick={onClose}>
            <span className="text-[clamp(14px,5vh,20px)] font-bold text-black">CLICK TO CLOSE</span>
          </button>
        </div>

        <div className="absolute right-0 bottom-0 left-0 box-border flex h-20 items-center justify-center gap-5 border-t-[10px] border-[#1D293D] bg-[#000109] p-[15px]">
          <button type="button" aria-label="Previous briefing page" onClick={handlePrevious} className="flex h-[50px] w-[50px] cursor-pointer items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-[#dfebeb] text-[28px] font-bold text-black">←</button>
          <div className="flex gap-3" aria-label={`Briefing page ${currentPage + 1} of ${pages.length}`}>
            {pages.map((briefingPage, index) => (
              <button key={briefingPage.id} type="button" aria-label={`Go to ${briefingPage.title}`} onClick={() => setCurrentPage(index)} className={`h-4 w-4 cursor-pointer rounded-full border-2 border-[#dfebeb] transition-all duration-300 ${currentPage === index ? 'bg-[#dfebeb]' : 'bg-[#666666]'}`} />
            ))}
          </div>
          <button type="button" aria-label="Next briefing page" onClick={handleNext} className="flex h-[50px] w-[50px] cursor-pointer items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-[#dfebeb] text-[28px] font-bold text-black">→</button>
        </div>
      </div>
    </div>
  );
}

export default function D1Brief(props: D1BriefProps) {
  return <D1BriefContent key={`${props.stand}:${props.sid}:${props.runway}`} {...props} />;
}
