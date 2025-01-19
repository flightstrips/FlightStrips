import { CurrentUTC } from "@/app/helpers/time";
import MRKBTN from "./MRKBTN";
import TRFBRN from "./TRFBRN";
import REQBTN from "./REQBTN";
import ATIS from "./ATIS";
import HOMEBTN from "./HOMEBTN";

export default function CommandBar() {
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
                    <span className="bg-[#212121]  w-18 p-2">
                        1015
                    </span>
                    <span className="bg-white text-black w-12 p-2 mx-2 text-center">
                        D
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
                <div className="w-32 bg-[#e4e4e4] text-black flex items-center justify-center p-3">
                    <CurrentUTC />
                </div>
            </div>
        </div>);
}