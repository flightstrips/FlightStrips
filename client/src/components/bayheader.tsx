import React from 'react'; // we need this to make JSX compile



export function BayHeader( props : any) {
    var msg = props.msg
    return(
        <div className={`${msg ? 'bg-[#285A5C]' : 'bg-slate-800'} 'w-full h-12  text-white font-bold flex items-center justify-between'`}>
            <p className='ml-2 text-xl uppercase'>{props.title}</p>
            <div className='flex flex-row'>
                {props.buttons}
            </div>

        </div>
    );
}

/*
                <button className='flex bg-gray-700 border-gray-300 border-2 w-20 h-4/5 text-white justify-center items-center font-bold text-xl mr-1'>
                    NEW
                </button>
                <button className='flex bg-gray-700 border-gray-300 border-2 w-32 h-4/5 text-white justify-center items-center font-bold text-xl mr-1'>
                    PLANNED
                </button>
*/