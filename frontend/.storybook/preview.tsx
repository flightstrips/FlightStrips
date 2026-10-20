import type { Preview } from '@storybook/react-vite';
import { MemoryRouter } from 'react-router';
import { TooltipProvider } from '../src/components/ui/tooltip';
import '../src/index.css';
import './preview.css';

const preview: Preview = {
  tags: ['autodocs'],
  initialGlobals: { backgrounds: { value: 'light' } },
  parameters: {
    layout: 'padded',
    backgrounds: { options: {
      light: { name: 'Light', value: '#ffffff' },
      board: { name: 'Controller board', value: '#555355' },
    } },
    options: { storySort: { order: ['Start here', 'Shared controls', 'Flight strips', 'Operations'] } },
    controls: { expanded: true },
    viewport: { options: {
      controller: { name: 'Controller display', styles: { width: '1920px', height: '1080px' } },
      laptop: { name: 'Laptop', styles: { width: '1366px', height: '768px' } },
      tablet: { name: 'Tablet', styles: { width: '768px', height: '1024px' } },
      phone: { name: 'Phone', styles: { width: '390px', height: '844px' } },
    } },
  },
  decorators: [(Story) => <MemoryRouter><TooltipProvider><Story /></TooltipProvider></MemoryRouter>],
};

export default preview;
