import { MainNavigation } from "./components/MainNavigation";
import Image from "next/image";


export default function Home() {
  return (
    <div style={{fontFamily: "Plus Jakarta Sans"}} className="bg-[#003d48] w-scren h-full min-h-screen text-white ">
      <nav className="flex h-20 items-center justify-between w-1/2 mx-auto">
        <h1 className="font-semibold text-xl">FlightStrips</h1>
        <MainNavigation />
      </nav>
      <section className="w-full h-[60rem] flex justify-between items-center px-32">
        <div>
          <h2 className="text-3xl"><span className="font-semibold text-6xl">FlightStips</span><br/>Experience next-gen strip management with </h2>
        </div>
        <Image src="/fsdemo.png" width="850" height="478" alt="fsdemo" />
      </section>
      <footer className="h-48 bg-white w-full flex  justify-around text-[#003d48]">
        <div className="p-1 aspect-video w-64 flex justify-center items-center">
          <span className="font-semibold text-2xl p-2">
            FlightStrips
          </span>
        </div>
        <div className="w-[32rem] flex gap-12 items-center">
          <section>
            <ul className="list-disc list-inside">
              <li className="list-none font-semibold">Getting Started</li>
              <li>Introduction</li>
              <li>Installation</li>
              <li>Development</li>
              <li>Documentation</li>
            </ul>
          </section>
          <section>
            <ul className="list-disc list-inside">
              <li className="list-none font-semibold">Features</li>
              <li>PDC</li>
              <li>BARS</li>
              <li>vACDM 2.0</li>
            </ul>
          </section>
          <section>
            <ul className="list-disc list-inside">
              <li className="list-none font-semibold">About</li>
              <li>Us</li>
              <li>License</li>
              <li>Contact</li>
            </ul>
          </section>

        </div>
      </footer>

    </div>
  );
}
