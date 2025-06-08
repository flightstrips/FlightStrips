export function Message({ children }: { children?: React.ReactNode }) {

    return (
        <div className="w-[calc(100&-0.25rem)]] h-14 bg-primary border-2 border-gray-100 text-gray-100 flex m-1 select-none">
            <div className={`border-2 select-none border-primary h-full justify-center items-center font-bold bg-slate-50 text-gray-600 min-w-10 w-fit flex`} style={{borderRightWidth: 1}}>
                GW
            </div>
            <div className="w-full h-full p-1 break-words line-clamp-2">
                {children}
            </div>
            <button type="submit" className="text-2xl border-[2px] aspect-square border-gray-100 m-2 flex justify-center items-center">
                X
            </button>
        </div>
    )
}