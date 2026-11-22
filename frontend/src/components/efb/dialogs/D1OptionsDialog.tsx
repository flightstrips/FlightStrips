import { useEffect, useId, useRef, useState } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import flytampaCover from '../../../assets/efb/scenery-flytampa.jpg';
import simnordCover from '../../../assets/efb/scenery-simnord.webp';

type SceneryId = 'flytampa' | 'simnord' | 'default';

interface D1OptionsDialogProps {
  isOpen: boolean;
  onClose: () => void;
  // Read and declare the pilot's scenery. FlightStrips avoids stands the
  // declared scenery lacks when assigning a stand.
  loadScenery?: () => Promise<string | null>;
  saveScenery?: (scenery: SceneryId) => Promise<unknown>;
}

interface SceneryOption {
  id: SceneryId;
  name: string;
  purchaseUrl: string | null;
  gsxUrl: string | null;
  cover: string | null;
}

const SCENERY_OPTIONS: SceneryOption[] = [
  {
    id: 'flytampa', name: 'FlyTampa Copenhagen',
    purchaseUrl: 'https://www.flytampa.org/ekch.html',
    gsxUrl: 'https://flightsim.to/addon/97152/flytampa-ekch-gsx-profile-official-vatsca-publication',
    cover: flytampaCover,
  },
  {
    id: 'simnord', name: 'SimNord Copenhagen',
    purchaseUrl: 'https://www.simnord.com/en/product/ekch-copenhagen-airport-msfs-2020-2024/',
    gsxUrl: 'https://flightsim.to/addon/114611/simnord-ekch-gsx-profile-official-vatsca-publication',
    cover: simnordCover,
  },
  { id: 'default', name: 'Default', purchaseUrl: null, gsxUrl: null, cover: null },
];

function isSceneryId(value: unknown): value is SceneryId {
  return SCENERY_OPTIONS.some((option) => option.id === value);
}

const linkBoxClassName = 'flex w-[17%] shrink-0 items-center justify-center rounded-md border-2 border-[#1A475F] bg-[#011328] text-center text-[clamp(10px,1.3vw,20px)] font-bold leading-tight text-white no-underline';

export default function D1OptionsDialog({ isOpen, onClose, loadScenery, saveScenery }: D1OptionsDialogProps) {
  const [scenery, setScenery] = useState<SceneryId | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(Boolean(loadScenery));
  const [saving, setSaving] = useState(false);
  const pendingSave = useRef<Promise<unknown> | null>(null);
  const previousFocus = useRef<HTMLElement | null>(null);
  const sceneryGroup = useId();

  useEffect(() => {
    if (!isOpen) return;
    setLoading(Boolean(loadScenery));
    if (!loadScenery) return;
    let current = true;
    setError(null);
    const load = async () => {
      // A reopened dialog must load the value after any outstanding save.
      if (pendingSave.current) await pendingSave.current.catch(() => undefined);
      if (!current) return;
      try {
        const value = await loadScenery();
        if (current) setScenery(isSceneryId(value) ? value : null);
      } catch {
        if (current) setError('Could not load your scenery choice.');
      } finally {
        if (current) setLoading(false);
      }
    };
    void load();
    return () => { current = false; };
  }, [isOpen, loadScenery]);

  const selectScenery = async (value: SceneryId) => {
    if (loading || pendingSave.current) return;
    const previous = scenery;
    setScenery(value);
    setError(null);
    if (!saveScenery) return;
    setSaving(true);
    try {
      const save = saveScenery(value);
      pendingSave.current = save;
      await save;
    } catch {
      setScenery(previous);
      setError('Could not save your scenery choice. Try again.');
    } finally {
      pendingSave.current = null;
      setSaving(false);
    }
  };

  return (
    <Dialog.Root open={isOpen} onOpenChange={(open) => { if (!open) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-[1000] bg-black/70" />
        <Dialog.Content
          aria-modal="true"
          aria-describedby={undefined}
          className="fixed top-1/2 left-1/2 z-[1001] flex aspect-[3/2] max-h-[85dvh] w-[95%] -translate-x-1/2 -translate-y-1/2 items-center justify-center overflow-hidden rounded-lg border-2 border-[#011328] bg-[#000109]"
          onOpenAutoFocus={() => {
            previousFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
          }}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            previousFocus.current?.focus();
          }}
        >
          <Dialog.Title className="sr-only">Options</Dialog.Title>
          <Dialog.Close
            type="button"
            aria-label="Close options"
            className="absolute top-[2%] right-[2%] z-10 h-14 w-14 cursor-pointer rounded-md border-0 bg-[#1A475F] text-lg font-bold text-white"
          >
            X
          </Dialog.Close>

          <div className="flex h-full w-full items-center justify-center px-[6%]">
            <section aria-labelledby="efb-scenery-heading" className="flex h-[60%] w-[80%] flex-col">
              <h2 id="efb-scenery-heading" className="mb-[3%] border-b border-[#38536E] pb-[1%] text-[clamp(12px,1.6vw,28px)] font-bold tracking-[0.08em] text-[#B8D9F0]">
                AIRPORT SCENERY
              </h2>
              <div role="radiogroup" aria-labelledby="efb-scenery-heading" aria-busy={loading || saving} className="grid min-h-0 flex-1 grid-rows-3 gap-[5%]">
                {SCENERY_OPTIONS.map((option) => {
                  const selected = scenery === option.id;
                  return (
                    <div key={option.id} className="flex min-h-0 items-stretch gap-[2%]">
                      <label className={`flex min-w-0 flex-1 cursor-pointer items-center gap-[3%] rounded-md border-2 px-[3%] text-left text-[clamp(12px,1.6vw,28px)] font-bold text-white has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-60 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-[#8ed7ec] ${selected ? 'border-[#41826E] bg-[#0B2A24]' : 'border-[#1A475F] bg-[#011328]'}`}>
                        <input
                          type="radio"
                          name={sceneryGroup}
                          value={option.id}
                          checked={selected}
                          disabled={loading || (saving && !selected)}
                          aria-disabled={saving || undefined}
                          onChange={() => void selectScenery(option.id)}
                          className="sr-only"
                        />
                        <span aria-hidden="true" className={`flex aspect-square h-[38%] shrink-0 items-center justify-center rounded-sm border-2 text-[clamp(12px,1.5vw,26px)] leading-none ${selected ? 'border-[#41826E] bg-[#41826E] text-white' : 'border-[#B8D9F0] text-transparent'}`}>
                          ✓
                        </span>
                        <span aria-hidden="true" className="relative aspect-square h-[80%] shrink-0 overflow-hidden rounded-sm">
                          {option.cover && <img src={option.cover} alt="" className="absolute inset-0 h-full w-full object-cover object-top" />}
                        </span>
                        <span className="min-w-0 leading-tight">{option.name}</span>
                      </label>
                      {option.purchaseUrl
                        ? <a href={option.purchaseUrl} target="_blank" rel="noreferrer" aria-label={`Buy ${option.name}`} className={linkBoxClassName}>BUY SCENERY</a>
                        : <span aria-hidden="true" className="w-[17%] shrink-0" />}
                      {option.gsxUrl
                        ? <a href={option.gsxUrl} target="_blank" rel="noreferrer" aria-label={`GSX profile for ${option.name}`} className={linkBoxClassName}>GSX PROFILE</a>
                        : <span aria-hidden="true" className="w-[17%] shrink-0" />}
                    </div>
                  );
                })}
              </div>
              <p role="status" className="mt-[2%] min-h-[1.5em] text-[clamp(10px,1.2vw,18px)] text-[#F0B8B8]">{error}</p>
            </section>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
