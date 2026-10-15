import type { Meta, StoryObj } from '@storybook/react-vite';
import { StripCell, SplitStripCell } from './StripCell';

const meta = {
  title: 'Flight strips/Shared cells', component: StripCell,
  decorators: [(Story) => <div className="flex h-14 bg-strip-dep-bg"><Story /></div>],
  args: { width: 160, children: 'SAS123' },
} satisfies Meta<typeof StripCell>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Single: Story = {};
export const Split: Story = { render: () => <SplitStripCell width={160} top="EGLL" bottom="A12" /> };
export const LongText: Story = { args: { children: 'LONG CALLSIGN AND DESTINATION' } };
