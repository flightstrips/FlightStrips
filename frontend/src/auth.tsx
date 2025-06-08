import { Button } from "./components/ui/button";

export default function Authentication() {
  return (
    <div className="min-h-svh w-screen flex items-center justify-center bg-gray-100">
        <div className="hidden xl:flex bg-primary w-full min-h-svh text-gray-100 text-6xl font-semibold items-center justify-center select-none">
            <h1>FlightStrips</h1>
        </div>
        <div className="bg-gray-100 w-full min-h-svh flex flex-col items-center justify-center text-primary relative">
            <h3 className="text-2xl font-semibold">ATC Strip Management</h3>
            <br />
            <div className="flex justify-between w-full max-w-2xl gap-4 px-4">
                <Button variant="default" size="lg" className="flex flex-col h-fit p-4 aspect-video w-48 font-semibold text-lg">
                    <img src="/VATSIM_Logo_White_No_Tagline_500px.png" alt="" className="h-12" />
                    Live
                </Button>
                <Button variant="default" size="lg" className="flex flex-col h-fit p-4 aspect-video w-48 font-semibold text-lg">
                    <img src="/VATSIM_Logo_White_No_Tagline_500px.png" alt="" className="h-12" />
                    Sweatbox
                </Button>
                <Button variant="default" size="lg" className="flex flex-col h-fit p-4 aspect-video w-48 font-semibold text-lg">
                    <img src="/VATSIM_Logo_White_No_Tagline_500px.png" alt="" className="h-12" />
                    Local Development
                </Button>
            </div>
            <div className="absolute bottom-4 left-1/2 transform -translate-x-1/2 text-xs text-gray-500 text-center">
                <p className="text-lg">FlightStrips is a free and open-source project.</p>
                <p className="text-lg">Support via Github</p>
            </div>
        </div>
    </div>
  );
}