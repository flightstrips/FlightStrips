import CurrentUTC from "@/helpers/time";
import MRKBTN from "./MRKBTN";
import TRFBRN from "./TRFBRN";
import REQBTN from "./REQBTN";
import ATIS from "./ATIS";
import HOMEBTN from "./HOMEBTN";
import GetMetar from "@/helpers/GetMetar"
import MetarHelper from "@/helpers/MetarHelper.tsx"


export default function CommandBar() {
    const metar = GetMetar({ icao: "EKCH" })
    return (
        <div className="h-16 w-screen bg-[#3b3b3b] flex justify-between text-white">
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
                <div className="flex w-fit text-2xl font-bold m-2 items-center justify-between">
                    <h1>
                        QNH
                    </h1>
                    <span className="bg-[#212121] w-18 p-2">
                        <MetarHelper metar={metar} style="qnh" />
                    </span>
                    <span className="bg-white text-black w-12 p-2 mx-2 text-center">
                        D
                    </span>
                    <span className="bg-white text-black w-32 p-2 mx-2 text-center text-xl">
                        <MetarHelper metar={metar} style="winds" />
                    </span>
                </div>
                <div className="flex w-fit text-2xl font-bold m-2 items-center justify-between">
                    <ATIS />
                </div>
            </div>
            <div className="flex items-center justify-center gap-1">
                <HOMEBTN />
                <TRFBRN />
                <MRKBTN />
                <REQBTN />
                <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                    X
                </button>
                <div className="w-32 bg-[#646464] flex items-center justify-center h-6/8 border-2">
                    <CurrentUTC />
                </div>
            </div>
        </div>);
}