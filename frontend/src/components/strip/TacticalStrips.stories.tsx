import type { Meta, StoryObj } from '@storybook/react-vite';
import { Bay } from '@/api/models';
import { StoryStore } from '@/stories/StoryStore';
import { TacticalRwyStrip } from './TacticalRwyStrip';
import { TacticalMemaidStrip } from './TacticalMemaidStrip';
import { TacticalCrossingStrip } from './TacticalCrossingStrip';
import { TacticalDeiceLaneStrip } from './TacticalDeiceLaneStrip';

const meta = {
  title: 'Flight strips/Tactical strips', component: TacticalRwyStrip,
  decorators: [(Story, context) => <StoryStore key={context.id}><div style={{ width: 'min(100%, 900px)', padding: 16, background: '#555355' }}><Story /></div></StoryStore>],
  args: { strip: {
    id: 42, session_id: 1, type: 'START', bay: Bay.Taxi, label: '22R',
    aircraft: 'SAS123', produced_by: 'EKCH_TWR', owner: 'EKCH_TWR',
    marked: false, timer_start: null, sequence: 1000, confirmed: false,
    confirmed_by: '', created_at: '2026-01-01T12:00:00Z',
  } },
} satisfies Meta<typeof TacticalRwyStrip>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Start: Story = {};
export const Land: Story = { args: { strip: { ...meta.args.strip, type: 'LAND', label: '22L' } } };
export const Confirmed: Story = { args: { strip: { ...meta.args.strip, confirmed: true, confirmed_by: 'EKCH_TWR' } } };
export const Marked: Story = { args: { strip: { ...meta.args.strip, marked: true } } };
export const OtherOwner: Story = { args: { strip: { ...meta.args.strip, owner: 'EKCH_GND' } } };
export const MemoryAid: Story = { render: ({ strip }) => <TacticalMemaidStrip strip={{ ...strip, type: 'MEMAID', label: 'CHECK RUNWAY BEFORE CLEARANCE' }} /> };
export const Crossing: Story = { render: ({ strip }) => <TacticalCrossingStrip strip={{ ...strip, type: 'CROSSING', label: 'CROSSING TRAFFIC' }} /> };
export const DeiceLane: Story = { render: ({ strip }) => <TacticalDeiceLaneStrip strip={{ ...strip, type: 'MEMAID', bay: Bay.DeIce, label: 'DEICE_HEADER:A:LANE 1 – 130.650MHz' }} /> };
