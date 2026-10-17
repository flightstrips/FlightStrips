import { Fragment, useState } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import { ChevronLeft, ChevronRight, X } from 'lucide-react';

interface BriefingPage {
  id: string;
  image: string;
  imageAlt: string;
  title: string;
  description: string[];
}

interface BriefingDialogProps {
  title: string;
  context: string;
  pages: BriefingPage[];
  currentPage: number;
  onPageChange: (page: number) => void;
  onClose: () => void;
}

export default function BriefingDialog({ title, context, pages, currentPage, onPageChange, onClose }: BriefingDialogProps) {
  const page = pages[currentPage];
  const [direction, setDirection] = useState<'forward' | 'backward'>('forward');
  const changePage = (index: number, nextDirection: typeof direction) => {
    if (index === currentPage) return;
    setDirection(nextDirection);
    onPageChange(index);
  };
  const previous = () => changePage((currentPage + pages.length - 1) % pages.length, 'backward');
  const next = () => changePage((currentPage + 1) % pages.length, 'forward');

  return (
    <Dialog.Root open onOpenChange={(open) => { if (!open) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-[1000] bg-black/75" />
        <Dialog.Content
          aria-label={title}
          onKeyDown={(event) => {
            if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
              event.preventDefault();
              if (event.key === 'ArrowLeft') previous();
              else next();
            }
          }}
          className="fixed top-1/2 left-1/2 z-[1001] flex h-[92dvh] w-[96vw] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-2xl border border-white/10 bg-[#000109] text-white shadow-2xl outline-none"
        >
          <header className="flex shrink-0 items-center justify-between gap-4 border-b border-white/10 bg-[#011328] px-5 py-3">
            <div className="min-w-0">
              <Dialog.Title className="text-[clamp(16px,2.3vh,26px)] font-bold">{title}</Dialog.Title>
              <Dialog.Description className="truncate text-[clamp(11px,1.5vh,16px)] text-[#B8D9F0]">{context}</Dialog.Description>
            </div>
            <Dialog.Close aria-label="CLICK TO CLOSE" className="flex shrink-0 cursor-pointer items-center gap-2 rounded-lg border border-white/15 px-3 py-2 text-sm font-bold hover:bg-[#1A475F] focus-visible:outline-2 focus-visible:outline-[#43C6E7]">
              <X aria-hidden="true" className="size-5" />
              CLOSE
            </Dialog.Close>
          </header>

          <div key={page.id} data-direction={direction} className="efb-briefing-page flex min-h-0 flex-1 flex-col md:flex-row">
            <div className="flex min-h-0 flex-1 items-center justify-center p-3">
              <img src={page.image} alt={page.imageAlt} className="max-h-full max-w-full rounded-xl object-contain shadow-[0_6px_24px_rgb(0_0_0_/_20%)]" />
            </div>
            <div className="max-h-[30%] shrink-0 overflow-y-auto border-t border-white/10 bg-[#011328]/50 p-[clamp(12px,2vh,24px)] md:max-h-none md:w-[28%] md:border-t-0 md:border-l">
              <div className="mb-2 text-xs font-semibold tracking-widest text-[#B8D9F0] uppercase">Step {currentPage + 1} of {pages.length}</div>
              <h2 aria-live="polite" className="mb-4 text-[clamp(17px,2.5vh,28px)] leading-tight font-bold">{page.title}</h2>
              {page.description.map((paragraph, index) => (
                <Fragment key={paragraph}>
                  {index > 0 && <hr className="my-4 border-white/10" />}
                  <p className="text-[clamp(13px,1.8vh,19px)] leading-relaxed text-slate-200">{paragraph}</p>
                </Fragment>
              ))}
            </div>
          </div>

          <nav aria-label="Briefing navigation" className="flex shrink-0 items-center justify-center gap-3 border-t border-white/10 bg-[#011328] px-4 py-3">
            <button type="button" aria-label="Previous briefing page" onClick={previous} className="flex min-h-11 cursor-pointer items-center gap-2 rounded-lg border border-white/15 bg-[#1A475F] px-4 py-2 font-bold hover:bg-[#245A74] focus-visible:outline-2 focus-visible:outline-[#43C6E7]">
              <ChevronLeft aria-hidden="true" className="size-6" />
              <span className="hidden sm:inline">PREVIOUS</span>
            </button>
            <div className="flex flex-wrap justify-center" aria-label={`Briefing page ${currentPage + 1} of ${pages.length}`}>
              {pages.map((briefingPage, index) => (
                <button key={briefingPage.id} type="button" aria-label={`Go to ${briefingPage.title}`} aria-current={currentPage === index ? 'step' : undefined} title={briefingPage.title} onClick={() => changePage(index, index > currentPage ? 'forward' : 'backward')} className="group flex size-7 cursor-pointer items-center justify-center rounded-full focus-visible:outline-2 focus-visible:outline-[#43C6E7]">
                  <span aria-hidden="true" className={`size-4 rounded-full border-2 border-[#B8D9F0]/70 ${currentPage === index ? 'bg-[#B8D9F0]' : 'bg-transparent group-hover:bg-[#1A475F]'}`} />
                </button>
              ))}
            </div>
            <button type="button" aria-label="Next briefing page" onClick={next} className="flex min-h-11 cursor-pointer items-center gap-2 rounded-lg border border-white/15 bg-[#1A475F] px-4 py-2 font-bold hover:bg-[#245A74] focus-visible:outline-2 focus-visible:outline-[#43C6E7]">
              <span className="hidden sm:inline">{currentPage === pages.length - 1 ? 'START AGAIN' : 'NEXT'}</span>
              <ChevronRight aria-hidden="true" className="size-6" />
            </button>
          </nav>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
