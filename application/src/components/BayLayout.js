import React from "react";
import { Commandstrip } from "./CommandStrip";
import { Header } from "./Header";

export const BayLayout = () => {
  return (
    <div className="h-screen">
      <div className="bg-[#A9A9A9] w-screen h-full flex justify-center justify-items-center shrink">
        <div className="bg-[#555355] w-full h-auto ml-0 mr-2">
          <Header headerName="OTHERS" />
        </div>
        <div className="bg-[#555355] w-full h-auto ml-1 mr-1.5">
          <Header headerName="SAS" />
          <div className="h-2/4"></div>
          <Header headerName="NORWEGIAN" />
        </div>
        <div className="bg-[#555355] w-full h-auto ml-1.5 mr-1">
          <Header headerName="CLEARED" />
          <div className="h-2/4"></div>
          <Header headerName="MESSAGES" TypeMSG />
        </div>
        <div className="bg-[#555355] w-full h-auto ml-2 mr-0">
          <Header headerName="PUSHBACK" />
          <div className="h-1/3"></div><Header headerName="TWY DEP" />
        </div>
      </div>
      <Commandstrip />
    </div>
  );
};
