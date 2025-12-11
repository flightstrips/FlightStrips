import { useIsMobile } from "@/hooks/use-mobile";
import { ArrowLeft } from "lucide-react";
import { NavLink } from "react-router";

export default function LiveStartPage() {
  const mobile = useIsMobile();

  if (mobile) {
    return (
      <main className="bg-gray-100 my-12 flex items-center justify-center relative">
        <div className="flex flex-wrap bg-gray-900 shadow-2xl">
          <div className="w-full py-10 px-12 text-white/90">
            <p className="font-bold text-white mb-4">
              You cannot run this application on mobile. <br />
              Mobiles are not supported.
            </p>
            <NavLink to="/app/dashboard" className="text-white underline">
              Click here to go back to dashboard
            </NavLink>
          </div>
        </div>
      </main>
    );
  }

  return (
    <main className="min-h-svh bg-gray-100 flex items-center justify-center">
      <div className="container px-4 mx-auto">
        <div className="flex flex-wrap bg-gray-900 shadow-2xl">
          <div className="w-full lg:w-1/2 3xl:w-1/3">
            <div className="py-24 px-12 lg:px-20 text-white/90">
              <NavLink
                className="flex mb-6 items-center text-white/75 hover:text-white/50"
                to="/app/dashboard"
              >
                <ArrowLeft size={24} />
                <span className="ml-4">Back to Dashboard</span>
              </NavLink>
              <h1>Live Start Page</h1>
              <p>To be added</p>
              <p>- Selector of airports.</p>
              <p>- Euroscope connection info</p>
            </div>
          </div>
          <div className=" my-auto w-full lg:w-1/2 3xl:w-2/3">
            <div className="flex flex-1 items-center p-10 bg-white">
              <div className="flex items-center justify-left w-full max-w-2xl gap-4 px-4">
                <NavLink to="/app/live/offline" className="">
                  <div className="border-2 border-dashed bg-gray-300 rounded-lg px-4 py-3 hover:border-gray-500 transition">
                    <h2 className="text-xl font-semibold">
                      Start Offline Session
                    </h2>
                  </div>
                </NavLink>
              </div>
            </div>
          </div>
        </div>
      </div>
    </main>
  );
}
