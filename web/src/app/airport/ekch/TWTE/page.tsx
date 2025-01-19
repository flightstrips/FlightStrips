import CommandBar from "../../../components/commandbar/CommandBar";

export default function Home() {
  return (
    <>
      <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2 aspect-video">
        <div className="w-1/4 h-full bg-[#555355]">

        </div>
        <div className="w-1/4 h-full bg-[#555355]">

        </div>
        <div className="w-1/4 h-full bg-[#555355]">

        </div>
        <div className="w-1/4 h-full bg-[#555355]">
        
        </div>
      </div>
      <CommandBar />
    </>
  );
}
