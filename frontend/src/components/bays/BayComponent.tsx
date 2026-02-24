import { FlightStrip } from "../strip/FlightStrip";
import { Message } from "../Message";
import CommandBar from "../commandbar/CommandBar";

export default function BayComponent() {
    return (<>
        <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2 aspect-video">
            <div className="w-1/4 h-full bg-[#555355]">
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        OTHERS
                    </span>
                    <span className="flex gap-2">
                        <button className="bg-[#646464] text-white font-bold text-lg px-4 border-2 border-white active:bg-[#424242]">
                            NEW
                        </button>
                        <button className="bg-[#646464] text-white font-bold text-lg px-4 border-2 border-white active:bg-[#424242]">
                            PLANNED
                        </button>
                    </span>
                </div>
                <div className="h-[calc(100%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                    <FlightStrip callsign="DAT3676" standChanged taxiway="D" holdingPoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsign="BAW1234" standChanged taxiway="A" holdingPoint="B1" destination={'EGLL'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsign="DLH5678" standChanged taxiway="B" holdingPoint="C2" destination={'EDDF'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsign="AFR4321" standChanged taxiway="C" holdingPoint="D3" destination={'LFPG'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsign="KLM8765" standChanged taxiway="D" holdingPoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsign="RYR2345" standChanged taxiway="E" holdingPoint="F5" destination={'EIDW'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsign="EZY3676" standChanged taxiway="D" holdingPoint="E1" destination={'EGKK'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsign="WZZ1234" standChanged taxiway="A" holdingPoint="B1" destination={'LHBP'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsign="UAL5678" standChanged taxiway="B" holdingPoint="C2" destination={'KORD'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsign="AAL4321" standChanged taxiway="C" holdingPoint="D3" destination={'KJFK'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsign="QFA8765" standChanged taxiway="D" holdingPoint="E4" destination={'YSSY'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsign="ANA2345" standChanged taxiway="E" holdingPoint="F5" destination={'RJTT'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsign="JAL3676" standChanged taxiway="D" holdingPoint="E1" destination={'RJAA'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsign="SIA1234" standChanged taxiway="A" holdingPoint="B1" destination={'WSSS'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsign="THA5678" standChanged taxiway="B" holdingPoint="C2" destination={'VTBS'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsign="MAS4321" standChanged taxiway="C" holdingPoint="D3" destination={'WMKK'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsign="CXA8765" standChanged taxiway="D" holdingPoint="E4" destination={'VHHH'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsign="EVA2345" standChanged taxiway="E" holdingPoint="F5" destination={'RCTP'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsign="KAL3676" standChanged taxiway="D" holdingPoint="E1" destination={'RKSI'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsign="ETD1234" standChanged taxiway="A" holdingPoint="B1" destination={'OMAA'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsign="QTR5678" standChanged taxiway="B" holdingPoint="C2" destination={'OTHH'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsign="GIA4321" standChanged taxiway="C" holdingPoint="D3" destination={'WIII'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsign="VIR8765" standChanged taxiway="D" holdingPoint="E4" destination={'EGLL'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsign="BAW2345" standChanged taxiway="E" holdingPoint="F5" destination={'EGLL'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsign="DLH3676" standChanged taxiway="D" holdingPoint="E1" destination={'EDDF'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsign="AFR1234" standChanged taxiway="A" holdingPoint="B1" destination={'LFPG'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsign="KLM5678" standChanged taxiway="B" holdingPoint="C2" destination={'EHAM'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsign="RYR4321" standChanged taxiway="C" holdingPoint="D3" destination={'EIDW'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsign="EZY8765" standChanged taxiway="D" holdingPoint="E4" destination={'EGKK'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsign="WZZ2345" standChanged taxiway="E" holdingPoint="F5" destination={'LHBP'} stand={'A32'} tsat={'1500'} status="CLR"/>
                </div>
            </div>
            <div className="w-1/4 h-full bg-[#555355]">
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        SAS
                    </span>
                </div>
                <div className="h-[calc(50%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                <FlightStrip callsign="OYYSB" standChanged taxiway="D" holdingPoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsign="SAS1234" standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsign="SAS5678" standChanged taxiway="B" holdingPoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsign="SAS4321" standChanged taxiway="C" holdingPoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsign="SAS8765" standChanged taxiway="D" holdingPoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsign="SAS2345" standChanged taxiway="E" holdingPoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsign="SAS3676" standChanged taxiway="D" holdingPoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsign="SAS1234" standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsign="SAS5678" standChanged taxiway="B" holdingPoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsign="SAS4321" standChanged taxiway="C" holdingPoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsign="SAS8765" standChanged taxiway="D" holdingPoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsign="SAS2345" standChanged taxiway="E" holdingPoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'} status="CLR"/>
                </div>
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        NORWEGIAN
                    </span>
                </div>
                <div className="h-[calc(50%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                    <FlightStrip callsign="NSZ3676" standChanged taxiway="D" holdingPoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsign="NSZ1234" standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsign="NAX5678" standChanged taxiway="B" holdingPoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsign="NOZ4321" standChanged taxiway="C" holdingPoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsign="WIF8765" standChanged taxiway="D" holdingPoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsign="NOZ2345" standChanged taxiway="E" holdingPoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsign="NSZ3676" standChanged taxiway="D" holdingPoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsign="NSZ1234" standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsign="NAX5678" standChanged taxiway="B" holdingPoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsign="NOZ4321" standChanged taxiway="C" holdingPoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsign="WIF8765" standChanged taxiway="D" holdingPoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsign="NOZ2345" standChanged taxiway="E" holdingPoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'} status="CLR"/>
                </div>
            </div>
            <div className="w-1/4 h-full bg-[#555355]">
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        CLEARED
                    </span>
                </div>
                <div className="h-1/2 w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                <FlightStrip callsign="NSZ3676" clearances standChanged taxiway="D" holdingPoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLROK"/>
                <FlightStrip callsign="NSZ1234" clearances standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="CLROK"/>
                </div>
                <div className="bg-[#285a5c] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        MESSAGES
                    </span>
                </div>
                <div className="h-[calc(50%-6rem)] w-full bg-[#555355]">
                    <Message>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Nam tincidunt vitae enim eget porttitor. Suspendisse ultrices ullamcorper tortor, vitae condimentum lacus convallis at. </Message>
                    <Message><b>FLIGHTSTRIPS</b> has deteced that EKCH_DEL has logged off. You are in change of Delivery!</Message>
                    <Message>VFR Request LOW PASS rwy 12</Message>
                </div>
            </div>
            <div className="w-1/4 h-full bg-[#555355]">
                <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
                    <span className="text-[#393939] font-bold text-lg">
                        PUSHBACK
                    </span>
                </div>
                <div className="h-2/5 w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                    <FlightStrip callsign="NSZ1234" clearances standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsign="NSZ1234" clearances standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsign="NSZ1234" clearances standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsign="NSZ1234" clearances standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsign="NSZ1234" clearances standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                </div>
                <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
                    <span className="text-[#393939] font-bold text-lg">
                        TWY DEP
                    </span>
                </div>
                <div className="h-[calc(60%-5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                <FlightStrip callsign="NSZ1234" clearances standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsign="NSZ1234" clearances standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsign="NSZ1234" clearances standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsign="NSZ1234" clearances standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsign="NSZ1234" clearances standChanged taxiway="A" holdingPoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                </div>
            </div>
        </div>
        <CommandBar />
    </>) 
}