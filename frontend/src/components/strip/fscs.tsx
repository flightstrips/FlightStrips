import React from 'react' // we need this to make JSX compile

export function FSCS(props: any) {
  return (
    <div className="flex text-xl font-bold items-center pl-2 w-28 border-2 border-[#85B4AF]">
      {props.cs}
    </div>
  )
}
