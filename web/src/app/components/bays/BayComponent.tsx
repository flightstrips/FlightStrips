import { CurrentUTC } from "@/app/helpers/time";
import { FlightStrip } from "../strip/FlightStrip";

export default function BayComponent() {
    return (<>
        <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2 aspect-video">
            <div className="w-full h-full bg-[#555355]">
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
                <div className="h-[calc(100%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto">
                    <FlightStrip callsing="RYR2MY" clearances/>
                    <FlightStrip callsing="DLH2GH" />
                    <FlightStrip callsing="PHX124" />
                    <FlightStrip callsing="NSZ3676" clearances />
                    <FlightStrip callsing="SAS1988" />
                    <FlightStrip callsing="SAS22H" />
                    <FlightStrip callsing="SAS1244" />
                    <FlightStrip callsing="NSZ37A" />
                    <FlightStrip callsing="AUA30P" />
                    <FlightStrip callsing="ETD4EA" />
                    <FlightStrip callsing="EZS17AG" />
                    <FlightStrip callsing="RYR57KY" />
                    <FlightStrip callsing="NSZ3512" />
                    <FlightStrip callsing="SAS455" />
                    <FlightStrip callsing="EZY38RX" />
                </div>
            </div>
            <div className="w-full h-full bg-[#555355]">
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        SAS
                    </span>
                </div>
                <div className="h-1/2 w-full bg-[#555355]">
                    Lorem ipsum dolor sit amet, consectetur adipisicing elit. Fugit veniam minima laudantium distinctio nisi unde eveniet quibusdam similique, atque laboriosam asperiores ducimus nam maxime odit at sapiente. Similique, eveniet neque.
                </div>
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        NORWEGIAN
                    </span>
                </div>
                <div className="h-[calc(50%-5rem)] w-full bg-[#555355]">
                    Lorem ipsum dolor sit amet, consectetur adipisicing elit. Fugit veniam minima laudantium distinctio nisi unde eveniet quibusdam similique, atque laboriosam asperiores ducimus nam maxime odit at sapiente. Similique, eveniet neque.
                </div>
            </div>
            <div className="w-full h-full bg-[#555355]">
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        CLEARED
                    </span>
                </div>
                <div className="h-1/2 w-full bg-[#555355]">
                    Lorem ipsum dolor sit amet, consectetur adipisicing elit. Fugit veniam minima laudantium distinctio nisi unde eveniet quibusdam similique, atque laboriosam asperiores ducimus nam maxime odit at sapiente. Similique, eveniet neque.
                </div>
                <div className="bg-[#285a5c] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        MESSAGES
                    </span>
                </div>
                <div className="h-[calc(50%-5rem)] w-full bg-[#555355]">
                    Lorem ipsum dolor sit amet, consectetur adipisicing elit. Fugit veniam minima laudantium distinctio nisi unde eveniet quibusdam similique, atque laboriosam asperiores ducimus nam maxime odit at sapiente. Similique, eveniet neque.
                </div>
            </div>
            <div className="w-full h-full bg-[#555355]">
                <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
                    <span className="text-[#393939] font-bold text-lg">
                        PUSHBACK
                    </span>
                </div>
                <div className="h-2/5 w-full bg-[#555355]">
                    Lorem ipsum dolor sit amet, consectetur adipisicing elit. Fugit veniam minima laudantium distinctio nisi unde eveniet quibusdam similique, atque laboriosam asperiores ducimus nam maxime odit at sapiente. Similique, eveniet neque.
                </div>
                <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
                    <span className="text-[#393939] font-bold text-lg">
                        TWY DEP
                    </span>
                </div>
                <div className="h-[calc(60%-5rem)] w-full bg-[#555355]">
                    Lorem ipsum dolor sit amet, consectetur adipisicing elit. Fugit veniam minima laudantium distinctio nisi unde eveniet quibusdam similique, atque laboriosam asperiores ducimus nam maxime odit at sapiente. Similique, eveniet neque.
                </div>
            </div>
        </div>
        <div className="h-16 w-screen bg-[#3b3b3b] flex justify-between">
            <div className="h-full w-full flex">
                <div className="bg-[#1bff16] text-black w-32 flex justify-center items-center m-2 font-bold">
                    CLR DEL
                </div>
                <div className="flex w-32 text-2xl font-bold m-2 items-center justify-between">
                    <h1>
                        DEP
                    </h1>
                    <span className="bg-white text-black w-16 p-2">
                        22R
                    </span>
                </div>
                <div className="flex w-32 text-2xl font-bold m-2 items-center justify-between">
                    <h1>
                        ARR
                    </h1>
                    <span className="bg-white text-black w-16 p-2">
                        22L
                    </span>
                </div>
                <div className="flex w-32 text-2xl font-bold m-2 items-center justify-between">
                    <h1>
                        QNH
                    </h1>
                    <span className="bg-[#212121]  w-18 p-2">
                        1015
                    </span>
                </div>
            </div>
            <div className="flex items-center justify-center gap-1">
                <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                    TRF
                </button>
                <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                    MRK
                </button>
                <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                    REQ
                </button>
                <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                    X
                </button>
                <div className="w-32 bg-[#e4e4e4] text-black flex items-center justify-center p-3">
                    <CurrentUTC />
                </div>
            </div>
        </div>
    </>) 
}