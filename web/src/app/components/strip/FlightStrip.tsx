import React from 'react';
import { CLXBtn } from '../clxbtn';

type FlightStripProps = {
    callsing: string
    clearances: boolean
    standchanged?: boolean
    taxiway?: string
    holdingpoint?: string
    destination?: string
    stand?: string
    tsat?: string
    ctot?: string
}

const FlightStrip: React.FC<FlightStripProps> = (props) => {

    type BasePlateProps = {
        arrival?: boolean,
        callsing: string,
        clearances: boolean
        standchanged?: boolean
        taxiway?: string
        holdingpoint?: string
        destination?: string
        stand?: string
        tsat?: string
        ctot?: string
    }

    function Strip({ children }: { children?: React.ReactNode }) { 
        return (
            <div className={`w-fit h-12 bg-[#bef5ef] border-2 border-white text-black flex`}>
                {children}
            </div>
        )
    }

    function StripCLX() { 
        return (
            <Strip>
                <div className='border-2 border-[#85b4af] h-full min-w-24 w-fit font-bold' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                    <button className='active:bg-pink-500 active:border-2 active:border-l-0 active:border-t-0  w-full text-left pl-1 select-none'>{props.callsing}</button>
                </div>
                <button className='border-2 border-[#85b4af] h-full text-sm text-center min-w-16 w-fit select-none py-1' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                    <CLXBtn><span className='font-semibold'>{props.destination}</span><span>{props.stand}</span></CLXBtn>
                </button>
                <div className='border-2 border-[#85b4af] h-full text-sm text-center min-w-24 w-fit select-none flex justify-between px-1' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                    <span>EOBT</span>
                    <span>1312</span>
                </div>
                <div className='flex flex-col min-w-24 w-fit border-[#85b4af] border-2 h-full text-sm' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                        <div className='border-[#85b4af] h-1/2 w-full flex justify-between px-1' style={{borderBottomWidth: 1}}>
                            <span className='select-none'>
                                TSAT
                            </span>
                            <span className='select-none'>
                                {props.tsat}
                            </span>
                        </div>
                        <div className='border-[#85b4af] h-1/2 w-full flex justify-between px-1' style={{borderTopWidth: 1}}>
                            <span className='select-none'>
                                CTOT
                            </span>
                            <span className='select-none'>
                                {props.ctot}
                            </span>
                        </div>
                </div>
                
            </Strip>
        )
    }

    function BasePlate(props: BasePlateProps) {

        if (props.arrival) {
            return (
                <div className={`w-[90%] h-12 bg-[#fff28e] `}>

                </div>
            )
        } else {
            return (
                <div className={`w-fit h-12 bg-[#bef5ef] border-2 border-white text-black flex`}>
                    <div className={`border-2 select-none border-[#85b4af] h-full justify-center items-center font-bold bg-slate-50 text-gray-600 min-w-8 w-fit ${ props.clearances ? "flex" : "hidden"}`} style={{borderRightWidth: 1}}>
                        GW
                    </div>
                    <div className='border-2 border-[#85b4af] h-full min-w-24 w-fit font-bold' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                        <button className='active:bg-pink-500 active:border-2 active:border-l-0 active:border-t-0  w-full text-left pl-1 select-none'>{props.callsing}</button>
                    </div>
                    <div className='border-2 border-[#85b4af] h-full text-sm text-center min-w-16 w-fit select-none' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                        B738 <br/> EIESN
                    </div>
                    <div className='border-2 border-[#85b4af] h-full min-w-14 w-fit font-bold p-1 text-center select-none' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                        <span className={`${props.standchanged ? "text-blue-900" : "text-black"}`}>{props.stand}</span>
                    </div>
                    <div className='flex flex-col min-w-16 w-fit border-[#85b4af] border-2 h-full text-sm' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                        <div className='border-[#85b4af] h-1/2 w-full relative' style={{borderBottomWidth: 1}}>
                            <span className='z-0 absolute left-1/2 transform -translate-x-1/2 text-gray-400 opacity-25 select-none'>
                                TWY
                            </span>
                            <span className='z-10 absolute left-1/2 transform -translate-x-1/2 text-blue-900 font-semibold select-none'>
                                {props.taxiway}
                            </span>
                        </div>
                        <div className='border-[#85b4af] h-1/2 w-full relative' style={{borderTopWidth: 1}}>
                        <span className='z-0 absolute left-1/2 transform -translate-x-1/2 text-gray-400 opacity-25 select-none'>
                                HP
                            </span>
                            <span className='z-10 absolute left-1/2 transform -translate-x-1/2 text-blue-900 font-semibold select-none'>
                                {props.holdingpoint}
                            </span>
                        </div>
                    </div>
                    <div className='border-2 border-[#85b4af] h-full min-w-16 w-fit font-bold p-1 relative select-none' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                        22R
                        <div className='-top-[2px] -right-[1px] absolute  border-[#85b4af] w-4 h-4' style={{borderWidth: 1}}>

                        </div>
                    </div>
                </div>
            )
        }
    }

    return  <div>
                <BasePlate callsing={props.callsing} clearances={props.clearances} standchanged={props.standchanged} taxiway={props.taxiway} holdingpoint={props.holdingpoint}  destination={props.destination} stand={props.stand} tsat={props.tsat} ctot={props.ctot} />
                <StripCLX />
            </div>;
};

export { FlightStrip, StripCLX };