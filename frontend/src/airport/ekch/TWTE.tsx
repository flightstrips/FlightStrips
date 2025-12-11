import CommandBar from "@/components/refactor/commandbar/CommandBar";

export default function TWTE() {
  return (
    <>
      <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2 aspect-video">
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">FINAL</span>
          </div>
          <div className="h-[calc(35%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">RWY ARR</span>
          </div>
          <div className="h-[calc(25%-2.5rem)] w-full bg-[#212121]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">TWY ARR</span>
          </div>
          <div className="h-[calc(35%-2.5rem)] w-full bg-[#555355]"></div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">TWY DEP</span>
          </div>
          <div className="h-[calc(35%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">RWY DEP</span>
          </div>
          <div className="h-[calc(25%-2.5rem)] w-full bg-[#212121]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">AIRBORNE</span>
          </div>
          <div className="h-[calc(35%-2.5rem)] w-full bg-[#555355]"></div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">PUSHBACK</span>
          </div>
          <div className="h-[calc(20%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">DE-ICE</span>
          </div>
          <div className="h-[calc(15%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">CONTROLZONE</span>
          </div>
          <div className="h-[calc(25%-2.5rem)] w-full bg-[#212121]"></div>
          <div className="bg-[#285a5c] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">MESSAGES</span>
          </div>
          <div className="h-[calc(35%-2.5rem)] w-full bg-[#555355]"></div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">CLRDEL</span>
          </div>
          <div className="h-[calc(85%-2.5rem)] w-full bg-[#555355]"></div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">STAND</span>
          </div>
          <div className="h-[calc(15%-2.5rem)] w-full bg-[#555355]"></div>
        </div>
      </div>
      <CommandBar />
    </>
  );
}
