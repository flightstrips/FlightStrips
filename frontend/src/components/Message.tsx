export function Message({ children }: { children?: React.ReactNode }) {

    return (
        <div className="w-[calc(100&-0.25rem)]] h-14 bg-[#285a5c] border-2 border-white text-white flex m-1">
            <div className={`border-2 select-none border-[#85b4af] h-full justify-center items-center font-bold bg-slate-50 text-gray-600 min-w-10 w-fit flex`} style={{borderRightWidth: 1}}>
                GW
            </div>
            <div className="w-full h-full p-1 break-words line-clamp-2">
                {children}
            </div>
            <button type="submit" className="text-2xl border-[3px] aspect-square text-center py-1 border-white m-1">
                X
            </button>
        </div>
    )
}