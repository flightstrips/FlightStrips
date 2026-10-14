import type { Meta, StoryObj } from '@storybook/react-vite';
import { fn } from 'storybook/test';
import { AMANWarningPanel } from './AMANWarningPanel';

const meta = {
  title: 'Operations/Arrival warnings', component: AMANWarningPanel,
  args: {
    connectionState: 'connected', presentationStatus: 'ready',
    current: { snapshot: 'available', items: [] },
    flights: [{ callsign: 'SAS123' }], onNavigateToFlight: fn(() => true),
  },
} satisfies Meta<typeof AMANWarningPanel>;
export default meta;
type Story = StoryObj<typeof meta>;
export const NoWarnings: Story = {};
export const Warning: Story = { args: { current: { snapshot: 'available', items: [{
  id: 'sample-weather', source: 'technical_health', component: 'weather', severity: 'warning', code: 'stale', message: 'Weather input is stale',
}] } } };
export const SpacingConflict: Story = { args: { current: { snapshot: 'available', items: [{
  id: 'sample-spacing', source: 'sequence', severity: 'error', code: 'spacing_conflict', callsign: 'SAS123', message: 'Required spacing is unavailable',
}] } } };
export const Disconnected: Story = { args: { connectionState: 'disconnected' } };
export const Degraded: Story = { args: { presentationStatus: 'degraded' } };
export const Unavailable: Story = { args: { current: { snapshot: 'omitted', items: [] } } };
