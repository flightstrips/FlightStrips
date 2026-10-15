import type { Meta, StoryObj } from '@storybook/react-vite';
import { SIBox } from './SIBox';
import { StoryStore } from '@/stories/StoryStore';

const meta = {
  title: 'Flight strips/Ownership indicator', component: SIBox,
  decorators: [(Story, context) => <StoryStore key={context.id}><div style={{ display: 'flex', width: 80, height: 50 }}><Story /></div></StoryStore>],
  args: { callsign: 'SAS123', myPosition: 'EKCH_TWR', owner: 'EKCH_TWR', nextControllers: ['EKCH_GND'] },
} satisfies Meta<typeof SIBox>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Assumed: Story = {};
export const Concerned: Story = { args: { owner: '', nextControllers: ['EKCH_TWR'] } };
export const Unconcerned: Story = { args: { owner: 'EKCH_GND', nextControllers: [] } };
export const Sending: Story = { args: { transferFrom: 'EKCH_TWR', transferringTo: 'EKCH_GND' } };
export const Receiving: Story = { args: { owner: 'EKCH_GND', transferFrom: 'EKCH_GND', transferringTo: 'EKCH_TWR' } };
export const TagRequest: Story = { args: { transferFrom: 'EKCH_TWR', transferringTo: 'EKCH_GND', isTagRequest: true } };
export const Marked: Story = { args: { marked: true } };
