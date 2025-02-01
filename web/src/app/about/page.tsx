import { MainNavigation } from "../components/MainNavigation";
import MobileNav from "../components/MobileNavigation";

export default function Airport() {
  return ( 
    <div style={{fontFamily: "Plus Jakarta Sans"}} className="bg-[#003d48] w-scren h-full min-h-screen text-white overflow-hidden">
      <nav className="hidden md:flex h-20 items-center justify-between w-1/2 mx-auto">
        <h1 className="font-semibold text-xl">FlightStrips</h1>
        <MainNavigation />
      </nav>
      <nav className="flex items-center justify-between md:hidden ">
        <h1 className="font-semibold text-xl pl-6">FlightStrips</h1>
        <MobileNav />
      </nav>
      <section className="h-[35rem] w-screen text-white flex flex-col justify-center items-center">
        <h1 className="text-6xl py-4 font-semibold">About Us</h1>
        <p>Home / About Us</p>
      </section>
      <section className="bg-[#191919] w-screen min-h-screen h-fit flex items-center justify-center">
        <div className="w-full min-h-screen bg-[#191919] flex justify-center py-12">
          <div className="flex gap-2">
            <div className="w-8 h-32 bg-gradient-to-b from-[#003d48] to-transparent" />
            <h2 className="text-3xl font-semibold max-w-[24ch]">Our vision for a next generation strip management system</h2>
          </div>

        </div>
        <div className="w-full min-h-screen bg-[#191919] hidden md:flex">
b
        </div>
      </section>
      <footer className="h-48 bg-white w-full hidden md:flex justify-around text-[#003d48] ">
        <div className="p-1 aspect-video w-64 flex justify-center items-center">
          <span className="font-semibold text-2xl p-2">
            FlightStrips <br /> <span className="text-xs -mt-4 -pt-4">(Only for simulation)</span>
          </span>
        </div>
        <div className="w-[32rem] flex gap-12 items-center">
          <section>
            <ul className="list-disc list-inside">
              <li className="list-none font-semibold">Getting Started</li>
              <li><a href="">Introduction</a></li>
              <li><a href="">Installation</a></li>
              <li><a href="">Development</a></li>
              <li><a href="">Documentation</a></li>
            </ul>
          </section>
          <section>
            <ul className="list-disc list-inside">
              <li className="list-none font-semibold">Features</li>
              <li><a href="">PDC</a></li>
              <li><a href="">BARS</a></li>
              <li><a href="">vACDM</a></li>
            </ul>
          </section>
          <section>
            <ul className="list-disc list-inside">
              <li className="list-none font-semibold">About</li>
              <li><a href="">Us</a></li>
              <li><a href="">License</a></li>
              <li><a href="">Contact</a></li>
            </ul>
          </section>

        </div>
        </footer>
        <footer className="bg-white w-full h-64 text-[#003d48] flex flex-col gap-6 items-center md:hidden">
          <span className="font-semibold text-2xl pt-4">
              FlightStrips
          </span>
          <div className="flex justify-center gap-4">
            <section>
              <ul className="list-disc list-inside">
                <li className="list-none font-semibold">Getting Started</li>
                <li><a href="">Introduction</a></li>
                <li><a href="">Installation</a></li>
                <li><a href="">Development</a></li>
                <li><a href="">Documentation</a></li>
              </ul>
            </section>
            <section>
              <ul className="list-disc list-inside">
                <li className="list-none font-semibold">Features</li>
                <li><a href="">PDC</a></li>
                <li><a href="">BARS</a></li>
                <li><a href="">vACDM</a></li>
              </ul>
            </section>
            <section>
              <ul className="list-disc list-inside">
                <li className="list-none font-semibold">About</li>
                <li><a href="">Us</a></li>
                <li><a href="">License</a></li>
                <li><a href="">Contact</a></li>
              </ul>
            </section>
          </div>
        </footer>
    </div>
  ) }