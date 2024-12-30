import React from 'react';

type FlightStripProps = {
    callsing: string
    clearances?: boolean
}

const FlightStrip: React.FC<FlightStripProps> = (props) => {

    type BasePlateProps = {
        arrival?: boolean,
        callsing: string,
        clearances?: boolean
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
                    <div className={`border-2 border-[#85b4af] h-full justify-center items-center font-bold bg-slate-50 text-gray-600 min-w-8 w-fit ${ props.clearances ? "flex" : "hidden"}`} style={{borderRightWidth: 1}}>
                        GW
                    </div>
                    <div className='border-2 border-[#85b4af] h-full min-w-24 w-fit font-bold p-1' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                        {props.callsing}
                    </div>
                    <div className='border-2 border-[#85b4af] h-full text-sm text-center min-w-16 w-fit' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                        B738 <br/> EIESN
                    </div>
                    <div className='border-2 border-[#85b4af] h-full min-w-14 w-fit font-bold p-1 text-center' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                        F7
                    </div>
                    <div className='flex flex-col min-w-16 w-fit border-[#85b4af] border-2 h-full text-sm' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                        <div className='border-[#85b4af] h-1/2 w-full' style={{borderBottomWidth: 1}}>
                            1234
                        </div>
                        <div className='border-[#85b4af] h-1/2 w-full' style={{borderTopWidth: 1}}>
                            1234
                        </div>
                    </div>
                    <div className='border-2 border-[#85b4af] h-full min-w-16 w-fit font-bold p-1 relative' style={{borderRightWidth: 1, borderLeftWidth: 1}}>
                        22R
                        <div className='-top-[2px] -right-[1px] absolute  border-[#85b4af] w-4 h-4' style={{borderWidth: 1}}>

                        </div>
                    </div>
                </div>
            )
        }
    }

    return <div>
        <BasePlate callsing={props.callsing} clearances={props.clearances}/>
    </div>;
};

export { FlightStrip };