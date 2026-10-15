import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, userEvent, within } from 'storybook/test';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Dialog, DialogTrigger, DialogContent, DialogTitle, DialogDescription, DialogHeader, DialogClose } from '@/components/ui/dialog';
import { Sheet, SheetTrigger, SheetContent, SheetHeader, SheetTitle, SheetDescription } from '@/components/ui/sheet';
import { Accordion, AccordionItem, AccordionTrigger, AccordionContent } from '@/components/ui/accordion';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import { Skeleton } from '@/components/ui/skeleton';
import { Separator } from '@/components/ui/separator';
import { Carousel, CarouselContent, CarouselItem, CarouselNext, CarouselPrevious } from '@/components/ui/carousel';
import { NavigationMenu, NavigationMenuList, NavigationMenuItem, NavigationMenuLink } from '@/components/ui/navigation-menu';
import { SidebarProvider, Sidebar, SidebarContent, SidebarGroup, SidebarGroupLabel, SidebarMenu, SidebarMenuItem, SidebarMenuButton, SidebarTrigger } from '@/components/ui/sidebar';

const meta = { title: 'Shared controls/Examples' } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const FormFields: Story = {
  render: () => <div className="grid gap-4 max-w-sm">
    <Label htmlFor="callsign">Callsign</Label><Input id="callsign" placeholder="SAS123" />
    <Label htmlFor="disabled">Assigned runway (disabled)</Label><Input id="disabled" value="22R" disabled />
    <Label htmlFor="invalid">Invalid callsign</Label><Input id="invalid" aria-invalid="true" aria-describedby="error" defaultValue="?" />
    <p id="error">Enter a valid callsign.</p>
  </div>,
};
export const InformationCard: Story = {
  render: () => <Card className="max-w-sm"><CardHeader><CardTitle>Flight details</CardTitle><CardDescription>Sample departure</CardDescription></CardHeader><CardContent>SAS123 · A320 · Stand A12</CardContent><CardFooter><Button>View flight</Button></CardFooter></Card>,
};
export const ConfirmationDialog: Story = {
  render: () => <Dialog><DialogTrigger asChild><Button>Open confirmation</Button></DialogTrigger><DialogContent><DialogHeader><DialogTitle>Confirm transfer</DialogTitle><DialogDescription>Transfer SAS123 to the next controller?</DialogDescription></DialogHeader><DialogClose asChild><Button>Confirm transfer</Button></DialogClose></DialogContent></Dialog>,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(canvas.getByRole('button', { name: 'Open confirmation' }));
    await expect(page.getByRole('dialog')).toBeVisible();
    await userEvent.keyboard('{Escape}');
    await expect(page.queryByRole('dialog')).not.toBeInTheDocument();
  },
};
export const SidePanel: Story = {
  render: () => <Sheet><SheetTrigger asChild><Button>Open details panel</Button></SheetTrigger><SheetContent><SheetHeader><SheetTitle>Flight details</SheetTitle><SheetDescription>Check the close button, overlay and keyboard focus.</SheetDescription></SheetHeader></SheetContent></Sheet>,
};
export const ExpandableDetails: Story = {
  render: () => <Accordion type="single" collapsible className="max-w-md"><AccordionItem value="review"><AccordionTrigger>What should I check?</AccordionTrigger><AccordionContent>Check spacing, text wrapping and keyboard navigation.</AccordionContent></AccordionItem><AccordionItem value="states"><AccordionTrigger>Which states matter?</AccordionTrigger><AccordionContent>Open, closed, focused, disabled and long content.</AccordionContent></AccordionItem></Accordion>,
};
export const HelpTooltip: Story = {
  render: () => <Tooltip><TooltipTrigger asChild><Button variant="outline">Hover or focus for help</Button></TooltipTrigger><TooltipContent>Transfer to the next controller</TooltipContent></Tooltip>,
};
export const Loading: Story = {
  render: () => <div className="grid gap-3 max-w-sm" aria-label="Loading flight details"><Skeleton className="h-8 w-48" /><Separator /><Skeleton className="h-16 w-full" /></div>,
};
export const ContentCarousel: Story = {
  render: () => <Carousel className="max-w-xs mx-14"><CarouselContent>{['Departure', 'Arrival', 'Ground'].map(label => <CarouselItem key={label}><Card><CardContent className="p-12">{label}</CardContent></Card></CarouselItem>)}</CarouselContent><CarouselPrevious /><CarouselNext /></Carousel>,
};
export const Navigation: Story = {
  render: () => <NavigationMenu><NavigationMenuList>{['Overview', 'Components', 'Review guide'].map(label => <NavigationMenuItem key={label}><NavigationMenuLink href="#" className="block p-3">{label}</NavigationMenuLink></NavigationMenuItem>)}</NavigationMenuList></NavigationMenu>,
};
export const SideNavigation: Story = {
  parameters: { layout: 'fullscreen' },
  render: () => <SidebarProvider><Sidebar><SidebarContent><SidebarGroup><SidebarGroupLabel>FlightStrips</SidebarGroupLabel><SidebarMenu>{['Overview', 'Departures', 'Arrivals'].map((label, index) => <SidebarMenuItem key={label}><SidebarMenuButton isActive={index === 0}>{label}</SidebarMenuButton></SidebarMenuItem>)}</SidebarMenu></SidebarGroup></SidebarContent></Sidebar><main className="p-4"><SidebarTrigger /><p>Toggle the sidebar or use a narrow viewport to review the mobile panel.</p></main></SidebarProvider>,
};
