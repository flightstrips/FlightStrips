import React from "react";
import { Commandstrip } from "../components/CommandStrip";
import { Header } from "../components/Header";
import { MSGModal } from "../components/MSGModal";

export const CTower = () => {
  return (
    <div className="h-screen">
      <div className="bg-[#A9A9A9] w-screen h-full flex justify-center justify-items-center shrink">
        <div className="bg-[#555355] w-full h-auto ml-0 mr-2">
          <Header headerName="FINAL" />
          <div className="h-1/5"></div>
          <Header headerName="RWY ARR"/>
          <div className="h-1/6"></div>
          <Header headerName="TWY ARR"/>
          <div className="h-2/5"></div>
          <Header headerName="STAND"/>
        </div>
        <div className="bg-[#555355] w-full h-auto ml-1 mr-1.5">
          <Header headerName="PUSHBACK" />
          <div className="h-[12.5%]"></div>
          <Header headerName="TWY DEP" />
          <div className="h-3/6"></div>
          <Header headerName="RWY DEP" />
          <div className="h-[12.5%]"></div>
          <Header headerName="AIRBORE" />
        </div>
        <div className="bg-[#555355] w-full h-auto ml-1.5 mr-1">
          <Header headerName="CONTROL ZONE" />
          <div className="h-2/4"></div>
          <Header headerName="MESSAGES" TypeMSG buttons={<MSGModal/>}/>
        </div>
        <div className="bg-[#555355] w-full h-auto ml-2 mr-0">
          <Header headerName="CLR DEL" />
          <div className="h-4/5"></div>
          <Header headerName="DE-ICE" />
        </div>
      </div>
      <Commandstrip />
    </div>
  );
};
