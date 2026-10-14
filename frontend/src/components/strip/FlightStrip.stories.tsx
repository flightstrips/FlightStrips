import type { Meta, StoryObj } from '@storybook/react-vite';
import { FlightStrip } from './FlightStrip';
import { StoryStore } from '@/stories/StoryStore';
import { Bay } from '@/api/models';

const meta = {
  title: 'Flight strips/Flight strip', component: FlightStrip,
  decorators: [(Story, context) => <StoryStore key={context.id}><div style={{ width: 'min(100%, 900px)', padding: 16, background: '#555355' }}><Story /></div></StoryStore>],
  parameters: {
    controls: { include: ['status', 'callsign', 'destination', 'origin', 'stand', 'aircraftType', 'runway', 'marked', 'pdcStatus', 'owner'] },
    docs: { description: { component: 'Production strip layouts. Use a desktop viewport to check spacing and text. Actions are recorded, but server responses are not simulated.' } },
  },
  args: {
    status: 'CLR', callsign: 'SAS123', destination: 'EGLL', origin: 'EKCH',
    stand: 'A12', aircraftType: 'A320', squawk: '1234', assignedSquawk: '1234',
    sid: 'ODN2B', runway: '22R', clearedAltitude: 5000,
    owner: 'EKCH_TWR', myPosition: 'EKCH_TWR', nextControllers: ['EKCH_GND'],
    fullWidth: true, selectable: true,
  },
  argTypes: { status: { control: 'select', options: ['CLR', 'CLROK', 'HALF', 'PUSH', 'ARR', 'FINAL-ARR', 'CLX-HALF', 'TAXI-DEP', 'TWY-DEP'] } },
} satisfies Meta<typeof FlightStrip>;
export default meta;
type Story = StoryObj<typeof meta>;
export const AwaitingClearance: Story = {};
export const Cleared: Story = { args: { status: 'CLROK' } };
export const Pushback: Story = { args: { status: 'PUSH' } };
export const Arrival: Story = { args: { status: 'ARR', arrival: true, bay: Bay.TwyArr } };
export const FinalArrival: Story = { args: { status: 'FINAL-ARR', arrival: true } };
export const ApronTaxi: Story = { args: { status: 'TAXI-DEP' } };
export const TowerTaxi: Story = { args: { status: 'TWY-DEP' } };
export const Compact: Story = { args: { status: 'HALF' } };
export const CompactClearance: Story = { args: { status: 'CLX-HALF' } };
export const Selected: Story = { args: { marked: true } };
export const PdcCleared: Story = { args: { pdcStatus: 'CLEARED' } };
export const Unowned: Story = { args: { owner: '', nextControllers: ['EKCH_TWR'] } };
export const LongCallsign: Story = { args: { callsign: 'SCANDINAVIAN1234' } };
export const MissingOptionalData: Story = { args: { stand: '', destination: '', aircraftType: '', sid: '', squawk: '', assignedSquawk: '' } };
