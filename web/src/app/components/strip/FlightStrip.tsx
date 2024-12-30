import React from 'react';

const FlightStrip: React.FC = () => {

    type BasePlateProps = {
        arrival?: boolean,
    }

    function BasePlate(props: BasePlateProps) {

        if (props.arrival) {
            return (
                <div className={`w-[90%] h-12 bg-[#fff28e] `}>

                </div>
            )
        } else {
            return (
                <div className={`w-[95%] h-12 bg-[#bef5ef] border-2 border-white text-black flex`}>
                    <div className='border-2 border-[#85b4af] h-full w-[10%] flex justify-center items-center font-bold'>
                        GW
                    </div>
                    <div className='border-2 border-[#85b4af] h-full w-[26%] font-bold p-1'>
                        RYR1EB
                    </div>
                    <div className='border-2 border-[#85b4af] h-full text-sm text-center w-[16%]'>
                        B738 <br/> EIESN
                    </div>
                    <div className='border-2 border-[#85b4af] h-full w-[16%] font-bold p-1 text-center'>
                        F7
                    </div>
                    <div className='flex flex-col w-[16%] border-[#85b4af] h-full text-sm'  style={{borderWidth: 1}}>
                        <div className='border-[#85b4af] h-1/2 w-full' style={{borderWidth: 1}}>
                            1234
                        </div>
                        <div className='border-[#85b4af] h-1/2l w-full' style={{borderWidth: 1}}>
                            1234
                        </div>
                    </div>
                    <div className='border-2 border-[#85b4af] h-full w-[16%] font-bold p-1 relative'>
                        22R
                        <div className='-top-[2px] -right-[1px] absolute border-2 border-[#85b4af] w-6 h-6'>

                        </div>
                    </div>
                </div>
            )
        }
    }

    return <div>
        <BasePlate />
    </div>;
};

export { FlightStrip };