import { LogIn } from "lucide-react";
import { Outlet } from "react-router";

// TODO: mobile navigation menu and move it to a component instead.
export default function MarketingLayout() {
  return (
    <div className="bg-gray-100 relative">
      <header className="bg-gray-900">
        <nav className="mr-12 py-8 px-8 bg-gray-200 overflow-hidden">
          <div className="flex w-full items-center justify-between">
            <a className="text-2xl inline-block self-center" href="#">
              <b>FlightStrips</b>
            </a>
            <ul className="relative flex justify-center items-center flex-wrap lg:flex-nowrap w-full lg:w-auto mt-4 lg:mt-0">
              <div className="absolute top-0 -left-8 h-32 -mt-20 w-px bg-gray-400"></div>
              <li className="mr-6 lg:mr-14 mb-2 lg:mb-0">
                <a
                  className="font-bold inline-flex items-center text-base text-gray-900 hover:text-gray-700"
                  href="#"
                >
                  <span>Getting started</span>
                </a>
              </li>
              <li className="mr-6 lg:mr-14 mb-2 lg:mb-0">
                <a
                  className="font-bold inline-flex items-center text-base text-gray-900 hover:text-gray-700"
                  href="#"
                >
                  <span>Features</span>
                </a>
              </li>
              <li className="mr-6 lg:mr-14 mb-2 lg:mb-0">
                <a
                  className="font-bold inline-flex items-center text-base text-gray-900 hover:text-gray-700"
                  href="#"
                >
                  <span>About Us</span>
                </a>
              </li>
              <li className="mr-6 mb-2 lg:mb-0">
                <a
                  className="3xl:hidden flex ml-auto items-center justify-center w-14 h-14 rounded-full bg-white hover:bg-gray-100"
                  href="#"
                >
                  <span>
                    <LogIn strokeWidth={2.5} />
                  </span>
                </a>
              </li>
            </ul>
          </div>
        </nav>
      </header>
      <Outlet />
      <footer>
        <div className="pt-10 pb-16 bg-gray-900">
          <div className="container px-4 mx-auto">
            <div className="flex flex-wrap items-start xl:items-center justify-center">
              <div className="w-1/2 xl:w-auto flex flex-wrap items-center justify-center xl:-mb-6">
                <a
                  className="inline-block w-full lg:w-auto text-sm text-white hover:text-gray-200 mb-6 mr-14"
                  href="#"
                >
                  Features
                </a>
                <a
                  className="inline-block w-full lg:w-auto text-sm text-white hover:text-gray-200 mb-6 mr-14"
                  href="#"
                >
                  Documentation
                </a>
                <a
                  className="inline-block w-full lg:w-auto text-sm text-white hover:text-gray-200 mb-6 mr-14"
                  href="#"
                >
                  Plans
                </a>
              </div>
              <div className="w-1/2 xl:w-auto flex flex-wrap items-center justify-center -mb-6">
                <a
                  className="inline-block w-full lg:w-auto text-sm text-white hover:text-gray-200 mb-6 mr-14"
                  href="#"
                >
                  About us
                </a>
                <a
                  className="inline-block w-full lg:w-auto text-sm text-white hover:text-gray-200 mb-6 mr-14"
                  href="#"
                >
                  License
                </a>
                <a
                  className="inline-block w-full lg:w-auto text-sm text-white hover:text-gray-200 mb-6"
                  href="#"
                >
                  Contact
                </a>
              </div>
            </div>
          </div>
        </div>
        <div className="py-12 text-center bg-gray-800">
          <div className="container px-4 mx-auto">
            <div className="flex sm:flex items-center justify-center mb-5">
              <a className="font-bold inline-block text-white" href="#">
                FlightStrips
              </a>
              <span className="block text-sm text-white font-light ml-2">
                © 2026 FlightStrips. All rights reserved.
              </span>
            </div>
            <p className="max-w-3xl mx-auto text-gray-400 text-xs font-light">
              Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do
              eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut
              enim ad minim veniam, quis nostrud exercitation ullamco laboris
              nisi ut aliquip ex ea commodo consequat.
            </p>
          </div>
        </div>
      </footer>
    </div>
  );
}
