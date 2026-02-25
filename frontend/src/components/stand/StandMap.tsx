import Stand from './Stand'

const stands: {
  label: string;
  col: number;
  row: number;
  spanX: number;
  spanY: number;
  status?: 'active' | 'reserved' | 'blue' | 'default';
}[] = [
  { label: 'A4', col: 10, row: 4, spanX: 2, spanY: 2, status: 'active' },
  { label: 'A50', col: 5, row: 8, spanX: 2, spanY: 2, status: 'reserved' },
  { label: 'Y4', col: 5, row: 5, spanX: 2, spanY: 1, status: 'blue' },
];

interface StandMapProps {
  onSelect?: (stand: string) => void;
}

export default function StandMap({ onSelect }: StandMapProps) {
  return (
    <div className="bg-gray-700 min-h-screen p-4">
      <div className="grid grid-cols-[repeat(30,_2rem)] grid-rows-[repeat(20,_3rem)] gap-1 relative h-full">
        {stands.map((s) => (
          <Stand
            key={s.label}
            label={s.label}
            style={{
              gridColumn: `${s.col} / span ${s.spanX}`,
              gridRow: `${s.row} / span ${s.spanY}`,
            }}
            status={s.status}
            position={''}
            onClick={() => onSelect?.(s.label)}
          />
        ))}
      </div>
    </div>
  )
}
