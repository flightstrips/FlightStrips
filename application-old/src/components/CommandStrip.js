import React from "react";
import { ATIS } from "./atis";
import { RunwayConfig } from "./RunwayConfig";
import ZuluTime from "./time";

export const Commandstrip = () => {

  return (
    <div className="bg-[#3C3C3C] w-screen h-16 absolute bottom-0 flex items-center pl-2 justify-between">
      <div className="pl-2 pr-2 flex items-center text-3xl font-bold">

        <div className="bg-[#1bff16] text-black w-40 h-12 text-center font-bold pl-4 pr-4 break-words text-base">
          TE-TW-TC<br/>APRON-DEL
        </div>

        <span className=" text-white ml-6 mr-2">DEP</span>
        <RunwayConfig runway="22R"/>
        <span className="  text-white ml-2 mr-2">ARR</span>
        <RunwayConfig runway="22R"/>
        <span className="  text-white ml-2 mr-2">QNH</span>

        <div className="bg-[#212121] text-white w-fit h-12 ml-4 mr-4 pl-4 pr-4  flex items-center text-center">
          1040
        </div>
        <ATIS />
        <div className="bg-white w-20 h-12 ml-4 mr-4 justify-center items-center flex pl-4 pr-4">
          D
        </div>
        <div className="bg-white w-450 h-12 ml-4 mr-4 justify-center items-center flex pl-4 pr-4 text-2xl">
          310/23G43KT
        </div>
      </div>
      <div className="pl-2 pr-2 flex items-center text-3xl font-extrabold">
      <button className="bg-[#646464] border-white border-2 w-fit h-12 pl-2 pr-2  ml-1 text-white">
          TRF
        </button>
        <button className="bg-[#646464] border-white border-2 w-fit h-12 pl-2 pr-2  ml-1 text-white">
          MRK
        </button>
        <button className="bg-[#646464] border-white border-2 w-fit h-12 pl-2 pr-2  ml-1 text-white">
          REQ
        </button>
        <button className="bg-[#646464] border-white border-2 w-16 h-12 pl-2 pr-2  ml-1 text-white">
          X
        </button>
        <div className="bg-white w-44 h-12 ml-2 mr-2 justify-center items-center flex pl-4 pr-4">
          <ZuluTime />
        </div>
      </div>
    </div>
  );
};
