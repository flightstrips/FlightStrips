import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, fn, userEvent, within } from 'storybook/test';
import { Button } from './button';

const meta = {
  title: 'Shared controls/Button', component: Button,
  args: { children: 'Confirm', onClick: fn() },
  argTypes: {
    variant: { control: 'select', options: ['default', 'destructive', 'outline', 'secondary', 'ghost', 'link', 'clx', 'trf', 'darkaction'] },
    size: { control: 'select', options: ['default', 'sm', 'lg', 'icon'] },
  },
} satisfies Meta<typeof Button>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {
  play: async ({ canvasElement, args }) => {
    await userEvent.click(within(canvasElement).getByRole('button', { name: 'Confirm' }));
    await expect(args.onClick).toHaveBeenCalled();
  },
};
export const Disabled: Story = { args: { disabled: true } };
export const LongLabel: Story = { args: { children: 'Confirm transfer to the next controller' } };
export const AllVariants: Story = {
  render: (args) => <div className="flex flex-wrap gap-4">{(['default', 'destructive', 'outline', 'secondary', 'ghost', 'link', 'clx', 'trf', 'darkaction'] as const).map(variant =>
    <div key={variant} className="w-40 h-12"><Button {...args} variant={variant}>{variant}</Button></div>)}</div>,
};
