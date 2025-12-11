import { ChevronRight } from "lucide-react";
import { NavLink, useNavigate } from "react-router";
import { useAuth0 } from "@auth0/auth0-react";
import { useEffect } from "react";

// TODO: change button design.
export default function AuthPage() {
  const { loginWithRedirect, isLoading, isAuthenticated } = useAuth0();
  const navigate = useNavigate();

  useEffect(() => {
    if (!isLoading) {
      if (isAuthenticated) {
        navigate("/app/dashboard");
      }
    }
  }, [isAuthenticated, isLoading, navigate]);

  return (
    <main className="min-h-svh bg-gray-100 flex items-center justify-center">
      <div className="container px-4 mx-auto">
        <div className="flex flex-wrap bg-gray-900 shadow-2xl">
          <div className="w-full lg:w-1/2 3xl:w-1/3">
            <div className="py-24 px-12 lg:px-20 text-white/90">
              <div>
                <NavLink
                  className="flex mb-6 items-center text-white/75 hover:text-white/50"
                  to="/"
                >
                  <svg
                    width="14"
                    height="11"
                    viewBox="0 0 14 11"
                    fill="none"
                    xmlns="http://www.w3.org/2000/svg"
                  >
                    <path
                      d="M6.10529 11L7.18516 10.0272L2.92291 6.1875L14 6.1875L14 4.8125L2.92291 4.8125L7.18516 0.972813L6.10529 -6.90178e-07L4.80825e-07 5.5L6.10529 11Z"
                      fill="currentColor"
                    ></path>
                  </svg>
                  <span className="ml-6">Back to frontpage</span>
                </NavLink>
                <h3 className="font-heading font-bold text-4xl mb-10">
                  FlightStrips - Login
                </h3>
                <p className="font-light mb-10">
                  ATC Strip Management for VATSIM controllers.
                </p>
              </div>
            </div>
          </div>
          <div className=" my-auto w-full lg:w-1/2 3xl:w-2/3">
            <div className="flex flex-1 items-center p-10 bg-white">
              <div className="flex items-center justify-center w-full max-w-2xl gap-4 px-4">
                {isLoading ? (
                  <div className="text-left">Loading...</div>
                ) : (
                  <button
                    onClick={() => {
                      loginWithRedirect({
                        authorizationParams: { connection: "vatsim-dev" },
                      });
                    }}
                    className="cursor-pointer bg-gray-700 text-white/90 hover:bg-gray-600 hover:text-white font-bold px-4 py-3.5 rounded inline-flex justify-between items-center w-full"
                  >
                    <div className="inline-flex space-x-5 items-center">
                      <img
                        src="/VATSIM_Logo_White_No_Tagline_500px.png"
                        alt=""
                        className="h-12 pointer-events-none"
                      />
                      <p>Local Development</p>
                    </div>
                    <ChevronRight size={48} />
                  </button>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </main>
  );
}
