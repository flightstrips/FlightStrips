import React from "react";
import { Commandstrip } from "../components/CommandStrip";
import { Header } from "../components/Header";
import { MSGModal } from "../components/MSGModal";

export const Apron = () => {
  return (
    <div className="h-screen">
      <div className="bg-[#A9A9A9] w-screen h-full flex justify-center justify-items-center shrink">
        <div className="bg-[#555355] w-full h-auto ml-0 mr-2">
          <Header headerName="MESSAGES" TypeMSG buttons={<MSGModal/>}/>
          <div className="h-1/4"></div>
          <Header headerName="RWY ARR" />
          <div className="h-2/4"></div>
          <Header headerName="STAND" />
        </div>
        <div className="bg-[#555355] w-full h-auto ml-1 mr-1.5">
          <Header headerName="TWY DEP" />
          <div className="h-2/4"></div>
          <Header headerName="TWY ARR" />
        </div>
        <div className="bg-[#555355] w-full h-auto ml-1.5 mr-1">
          <Header headerName="STARTUP" />
          <div className="h-2/5"></div>
          <Header headerName="PUSHBACK" />
        </div>
        <div className="bg-[#555355] w-full h-auto ml-2 mr-0">
          <Header headerName="CLR DEL" />
          <div className="h-2/3"></div>
            <Header headerName="DE-ICE" />
        </div>
      </div>
      <Commandstrip />
    </div>
  );
};
