import React from "react";
import { Commandstrip } from "../components/CommandStrip";
import { Header } from "../components/Header";
import { Strip } from "../components/Strip";
import { MSGModal } from "../components/MSGModal";
import { FindFlight } from "../components/buttons/FindFlight";
import { HalfStrip } from "../components/HalfStrip";

export const Delivery = () => {
  return (
    <>
      <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center shrink">
        <div className="bg-[#555355] w-full h-auto border-r-4 border-[#a9a9a9]">
          <Header headerName="OTHERS" buttons={<FindFlight />} />
            <div className="h-[calc(100%-2.5rem)] overflow-auto overflow-x-hidden">
            <Strip callsign="FIRST" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="VKG1332/t" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="LAST" ades="LGPR" stand="D3" eobt="1350" tsat="1400" ctot="1400" />
          </div>
        </div>
        <div className="bg-[#555355] h-full w-full border-l-4 border-r-4 border-[#a9a9a9]">
          <Header headerName="SAS" />
          <div className="h-[calc(60%-2.5rem)] overflow-auto overflow-x-hidden">
            <Strip callsign="FIRST" ades="EKYT" stand="B6" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="SAS7736" ades="LEPA" stand="B4" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="SAD43G" ades="ENGM" stand="B8" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="SAS418" ades="ESSA" stand="B7" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="SAS031" ades="LHBP" stand="B9" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="SAS446Y" ades="EGKK" stand="B16" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="SAS9909" ades="ENBR" stand="A12" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="SAS23" ades="EKBI" stand="A14" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="SAS928" ades="EKVG" stand="A11" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="SAS887" ades="KBOS" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="SAS42T" ades="EKYT" stand="B6" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="LAST" ades="LEPA" stand="B4" eobt="1350" tsat="1400" ctot="1400" />
          </div>
          <Header headerName="NORWEGIAN" />
          <div className="h-[calc(40%-2.5rem)] overflow-auto overflow-x-hidden">
            <Strip callsign="FIRST" ades="KBOS" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NAX948" ades="ENGM" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NOZ662" ades="EKYT" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS3096" ades="EKBI" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS38F" ades="EGKK" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NOZ662" ades="EKYT" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS3096" ades="EKBI" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS38F" ades="EGKK" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="LAST" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
          </div>
        </div>
        <div className="bg-[#555355] w-full h-auto border-l-4 border-r-4 border-[#a9a9a9]">
          <Header headerName="CLEARED" />
          <div className="h-[calc(50%-2.5rem)] overflow-auto overflow-x-hidden">
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
            <Strip callsign="NZS342" ades="ESSA" stand="B10" eobt="1350" tsat="1400" ctot="1400" />
          </div>
          <Header headerName="MESSAGES" TypeMSG buttons={<MSGModal />} />
          <div className="h-[calc(33%-2.5rem)] overflow-auto overflow-x-hidden">

          </div>
        </div>
        <div className="bg-[#555355] w-full h-auto border-l-2 border-[#a9a9a9]">
          <Header headerName="PUSHBACK" />
          <div className="h-[calc(33%-2.5rem)] overflow-auto overflow-x-hidden">
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
          </div>
          <Header headerName="TWY DEP" />
          <div className="h-[calc(66%-2.5rem)] overflow-auto overflow-x-hidden">
          <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
            <HalfStrip />
          </div>
        </div>
      </div>
      <Commandstrip />
    </>
  );
};
