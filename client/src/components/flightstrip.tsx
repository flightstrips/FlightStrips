import React from 'react'; // we need this to make JSX compile
import { EOBT } from './strip/eobt';
import { FSCS } from './strip/fscs';
import { DESSTD } from './strip/desstd';
import { TSATCTOT } from './strip/tsatctot';
import { OwnerBOX } from './strip/ownerbox';


export function FlightStrip( props : any ) {
    if (props.clearanceGranted) {
        return(
            <>
                <div className='flex border-white border-4 w-fit h-16 bg-[#BEF5EF]'>
                    <OwnerBOX />
                    <FSCS cs={props.cs} />
                    <DESSTD des={props.des} stand={props.stand} />
                    <EOBT time={props.time}/>
                    <TSATCTOT  TSAT={props.TSAT}/>
                </div>
    
            </>
        );
    } else {
        return(
            <>
                <div className='flex border-white border-4 w-fit h-16 bg-[#BEF5EF]'>
                    <FSCS cs={props.cs} />
                    <DESSTD des={props.des} stand={props.stand} />
                    <EOBT time={props.time}/>
                    <TSATCTOT  TSAT={props.TSAT}/>
                </div>
    
            </>
        );
    } 

}