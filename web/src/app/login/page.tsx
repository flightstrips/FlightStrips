import Image from "next/image";

export default function Home() {
  return (
    <div className="flex w-screen h-screen" style={{fontFamily: "Plus Jakarta Sans"}}>
      <div className="bg-[#003d48] text-white font-semibold h-full hidden md:flex md:w-1/2 justify-center items-center text-6xl flex-col gap-4">
        <h1>Fligthstrips</h1>
        <p className="text-base">Next-gen stip management</p>
      </div>
      <div className="bg-white h-full md:w-1/2 flex justify-center items-center gap-2 flex-col">
        <h2 className="text-2xl">Login</h2>
        <div className="flex gap-2 min-h-96 items-center">
          <button className="bg-[#005463] text-white rounded h-64 w-36 flex justify-center items-center">
            <Image src="/VATSIM_Logo_White_500px.png" width={100} height={50} alt="VATSIM Logo"/>
          </button>
          <button className="bg-[#005463] p-4 text-white rounded h-64 w-36">
            <h2 className="font-semibold">Local dev</h2>
          </button>
        </div>
        <p>OBS - Currently in active development</p>
        <p>The service may not always be stable</p>
      </div>
    </div>
  );
}
