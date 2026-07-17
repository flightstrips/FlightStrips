import { useState } from 'react';
import INTRO from '../../../assets/efb/INTRO.png';
import PUSHINTRO from '../../../assets/efb/PUSHINTRO.png';
import PUSH from '../../../assets/efb/PUSH.png';
import TAXIINIT from '../../../assets/efb/TAXIINIT.png';
import TAXINEXT from '../../../assets/efb/TAXINEXT.png';
import HANDOVER from '../../../assets/efb/TAXINEXT.png';
import SIDINIT from '../../../assets/efb/SIDINIT.png';
import SIDNEXT from '../../../assets/efb/SIDNEXT.png';
import COMPLETE from '../../../assets/efb/COMPLETE.png';

interface D1BriefPage {
  id: number;
  image: string;
  title: string;
  description: string;
}

interface D1BriefProps {
  isOpen: boolean;
  onClose: () => void;
  stand: string;
  sid: string;
}

export default function D1Brief({ isOpen, onClose, stand, sid }: D1BriefProps) {
  void stand;
  void sid;
  const [currentPage, setCurrentPage] = useState(0);

  // Static informational briefing content; it is not operational flight data.
  const briefingPages: D1BriefPage[] = [
    {
      id: 1,
      image: INTRO,
      title: 'Prepare your Flight',
      description: `As you prepare your flight it is important you have all the important information. Make sure you have DOWNLOADED the required content, Filed the TOBT, gotten the ATIS. All this available within the scope of your EFB`,
    },
    {
      id: 2,
      image: PUSHINTRO,
      title: 'Pushback Procedure',
      description: 'Most of the gates in EKCH has "RELEASE POINTS" which you must push to using your donwloaded GSX file.',
    },
    {
      id: 3,
      image: PUSH,
      title: 'What to expect?',
      description: 'Pushback to Release point J3 or pull forward to J4 is to be expected. A50 and Z5 are generally not used for these stands.',
    },
    {
      id: 4,
      image: TAXIINIT,
      title: 'Initial Taxi instructions',
      description: 'Via the standard Taxi Routes you can expect to hold short RWY30 via either TWY A or TWY F. On occastion when traffic demands TWY D can be used. Some stands will allow departure taxi via K2/K3 and taxiing down RWY12. Make sure you expect this instruction "Taxi via RWY12".',
    },
    {
      id: 5,
      image: TAXINEXT,
      title: 'Taxi with EKCH_TWR',
      description: 'TWR will allow you to cross RWY30 and assign you to holding points A1-A4, as requried for traffic sequencing. You are expected to be ready for departure when reaching holding point. Advise before hand if you are not, so an alternative holding point can be assigned.',
    },
    {
      id: 6,
      image: HANDOVER,
      title: 'Automatic Handover',
      description: 'Copenhagen has two different frequencies, and you MUST AUTOMATICALLY contact them when passing 1000ft. EKCH_TWR will NOT advise you of the frequency, he will simply state "Goodbye" indicating that its last contact with him. See the correct frequency on next page.  In Copenhagen Kastrup / NADP2 is used.',
    },
    {
      id: 7,
      image: SIDINIT,
      title: 'SID for your flight',
      description: 'AUTOMATICALLY contact passing 1000ft, Kastrup Departure on 124,980. Follow SID, but expect a potential direct from DEP. Expect further climb to FL190 when traffic allows. This SID is ONLY for JET Aircrafts',
    },
    {
      id: 7,
      image: SIDNEXT,
      title: 'Notice your SID',
      description: 'Remember initial climb is FL70. You must maintain 250kts or less below FL70, unless ATC tells you "Free Speed" or "High Speed Approved". Transition Altitude on this SID is 5000',
    },
    {
      id: 8,
      image: COMPLETE,
      title: 'We wish you a nice flight',
      description: 'Feedback can be given on cc.vatsim-scandinavia.org/feedback.',
    },
  ];

  if (!isOpen) return null;

  const handlePrevious = () => {
    setCurrentPage((prev) => (prev === 0 ? briefingPages.length - 1 : prev - 1));
  };

  const handleNext = () => {
    setCurrentPage((prev) => (prev === briefingPages.length - 1 ? 0 : prev + 1));
  };

  const handleDotClick = (index: number) => {
    setCurrentPage(index);
  };

  const page = briefingPages[currentPage];

  return (
    <div className="fixed inset-0 z-[1000] flex items-center justify-center bg-black/70" onClick={onClose}>
      <div
        role="dialog"
        aria-label="Departure briefing"
        className="relative flex aspect-[3/2] max-h-[85vh] w-[95%] overflow-hidden rounded-lg border-2 border-[#011328] bg-[#000109]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Left side - Picture */}
        <div className="flex h-full w-[80%] items-center justify-center border-r-2 border-[#000109] bg-[#000109] p-[10px]">
          <img
            src={page.image}
            alt={page.title}
            className="box-border h-full w-full border-2 border-[#011328] object-cover"
          />
        </div>

        {/* Right side - Text section */}
        <div className="flex h-full w-[20%] flex-col bg-[#000109]">
          {/* Explanatory Text (70% of right section) */}
          <div className="h-[70%] overflow-auto border-b-[10px] border-[#000109] p-5 text-[#dfebeb]">
            <h2 className="mt-0 mb-[15px] text-[clamp(16px,2.5vh,24px)] font-bold">
              {page.title}
            </h2>
            <p className="m-0 text-[clamp(12px,1.5vh,16px)] leading-[1.6] text-[#E0E0E0]">
              {page.description}
            </p>
          </div>

          {/* Close Button */}
          <button type="button" className="box-border flex h-[20%] cursor-pointer items-center justify-center border-[25px] border-[#000109] bg-[#dfebeb]" onClick={onClose}>
            <span className="text-[clamp(14px,5vh,20px)] font-bold text-black">
              CLICK TO CLOSE
            </span>
          </button>
        </div>

        {/* Bottom Navigation Bar */}
        <div className="absolute right-0 bottom-0 left-0 box-border flex h-20 items-center justify-center gap-5 border-t-[10px] border-[#1D293D] bg-[#000109] p-[15px]">
          {/* Left Arrow */}
          <button
            onClick={handlePrevious}
            className="flex h-[50px] w-[50px] cursor-pointer items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-[#dfebeb] text-[28px] font-bold text-black"
          >
            ←
          </button>

          {/* Dots */}
          <div className="flex gap-3">
            {briefingPages.map((_, index) => (
              <button
                key={index}
                onClick={() => handleDotClick(index)}
                className={`h-4 w-4 cursor-pointer rounded-full border-2 border-[#dfebeb] transition-all duration-300 ${currentPage === index ? 'bg-[#dfebeb]' : 'bg-[#666666]'}`}
              />
            ))}
          </div>

          {/* Right Arrow */}
          <button
            onClick={handleNext}
            className="flex h-[50px] w-[50px] cursor-pointer items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-[#dfebeb] text-[28px] font-bold text-black"
          >
            →
          </button>
        </div>
      </div>
    </div>
  );
}
