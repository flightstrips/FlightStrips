import CommandBar from "@/components/refactor/commandbar/CommandBar";

export default function Home() {
  return (
    <>
      <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2 aspect-video">
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-primary h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">MESSAGES</span>
          </div>
          <div className="h-[calc(30%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">FINAL</span>
          </div>
          <div className="h-[calc(25%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">RWY ARR</span>
          </div>
          <div className="h-[calc(30%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">STAND</span>
          </div>
          <div className="h-[calc(15%-2.5rem)] w-full bg-[#555355]"></div>
        </div>

        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">TWY DEP</span>
          </div>
          <div className="h-[calc(35%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="h-2 bg-[#a9a9a9]"></div>
          <div className="flex justify-center gap-2 pt-2 h-8">
            <button className="bg-[#393939] w-24 border-2 border-white text-white">
              TW
            </button>
            <button className="bg-[#393939] w-24 border-2 border-white text-white">
              TE
            </button>
            <button className="bg-[#393939] w-24 border-2 border-white text-white">
              GW
            </button>
            <button className="bg-[#393939] w-24 border-2 border-white text-white">
              GE
            </button>
          </div>
          <div className="h-[calc(30%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">TWY ARR</span>
          </div>
        </div>

        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">STARTUP</span>
          </div>
          <div className="h-[calc(60%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">PUSHBACK</span>
          </div>
          <div className="h-[calc(40%-2.5rem)] w-full bg-[#555355]"></div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">CLRDEL</span>
          </div>
          <div className="h-[calc(80%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">DE-ICE</span>
          </div>
          <div className="h-[calc(20%-2.5rem)] w-full bg-[#555355]"></div>
        </div>
      </div>
      <CommandBar />
    </>
  );
}
