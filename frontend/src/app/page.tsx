import { Package } from "lucide-react";

export default function LandingPage() {
  return (
    <main className="">
      <section className="relative">
        <div className="absolute -bottom-26 left-1/2 h-26 w-26 bg-gray-100 z-10 transform -translate-x-1/2">
          <div className="absolute bottom-0 left-1/2 mb-8">
            <div className="h-26 w-px bg-white" />
            <div className="h-18 w-px bg-gray-800" />
          </div>
        </div>
        <div className="relative bg-gray-900 overflow-hidden">
          <div className="hidden sm:block absolute top-0 left-0">
            <div className="w-8 h-96 bg-gray-200" />
            <div className="w-8 h-192 bg-white" />
          </div>
          <img
            className="hidden md:block absolute bottom-0 right-0 -mb-12 h-full w-1/2 pl-10 object-cover"
            src="/fsdemo.png"
            alt=""
          />
          <div className="container px-4 mx-auto">
            <div className="pt-28 pb-32">
              <div className="sm:ml-8 lg:ml-12">
                <div className="max-w-md 3xl:max-w-xl relative">
                  <div className="absolute bottom-0 -mb-12 -mr-10">
                    <svg
                      width={520}
                      height={100}
                      viewBox="0 0 520 100"
                      fill="none"
                      xmlns="http://www.w3.org/2000/svg"
                      className="transform -rotate-25"
                    >
                      <rect
                        x="10"
                        y="15"
                        width="500"
                        height="70"
                        rx="3"
                        stroke="#585353"
                        strokeWidth="2"
                      />

                      <line
                        x1="200"
                        y1="15"
                        x2="200"
                        y2="85"
                        stroke="#585353"
                        strokeWidth="1"
                      />
                      <line
                        x1="300"
                        y1="15"
                        x2="300"
                        y2="85"
                        stroke="#585353"
                        strokeWidth="1"
                      />
                      <line
                        x1="400"
                        y1="15"
                        x2="400"
                        y2="85"
                        stroke="#585353"
                        strokeWidth="1"
                      />
                    </svg>
                  </div>
                  <h1 className="text-5xl sm:text-6xl lg:text-7xl 3xl:text-8xl text-white mb-32 sm:mb-52 relative">
                    <span className="block">Experience</span>
                    <span>next-gen strip management</span>
                  </h1>
                </div>

                <div className="2xl:flex mb-18 items-center">
                  <div className="flex mb-4 2xl:mb-0 2xl:mr-12 items-center">
                    <svg
                      width={28}
                      height={28}
                      viewBox="0 0 28 28"
                      fill="none"
                      xmlns="http://www.w3.org/2000/svg"
                    >
                      <rect
                        opacity="0.1"
                        width={28}
                        height={28}
                        rx={5}
                        fill="#95A8FF"
                      />
                      <path
                        d="M18.9173 11.914L18.1189 11.086C18.0927 11.0587 18.0616 11.0371 18.0274 11.0223C17.9931 11.0076 17.9564 11 17.9193 11C17.8822 11 17.8455 11.0076 17.8113 11.0223C17.777 11.0371 17.7459 11.0587 17.7197 11.086L13.6372 15.5552L11.2971 13.1275C11.2435 13.0719 11.1708 13.0407 11.0949 13.0407C11.0191 13.0407 10.9463 13.0719 10.8927 13.1275L10.0837 13.9671C10.0301 14.0227 10 14.0981 10 14.1768C10 14.2554 10.0301 14.3308 10.0837 14.3865L13.4119 17.9178C13.4426 17.949 13.48 17.9724 13.5209 17.9861C13.5619 17.9997 13.6054 18.0033 13.6479 17.9965C13.6917 18.0044 13.7367 18.0014 13.7791 17.9877C13.8216 17.974 13.8603 17.9501 13.892 17.9178L18.9173 12.3281C18.9703 12.2732 19.0001 12.1987 19.0001 12.121C19.0001 12.0434 18.9703 11.9689 18.9173 11.9139V11.914Z"
                        fill="#95A8FF"
                      />
                    </svg>
                    <span className="ml-4 text-gray-500 font-light">
                      Quick setup
                    </span>
                  </div>
                  <div className="flex items-center">
                    <svg
                      width={28}
                      height={28}
                      viewBox="0 0 28 28"
                      fill="none"
                      xmlns="http://www.w3.org/2000/svg"
                    >
                      <rect
                        opacity="0.1"
                        width={28}
                        height={28}
                        rx={5}
                        fill="#95A8FF"
                      />
                      <path
                        d="M18.9173 11.914L18.1189 11.086C18.0927 11.0587 18.0616 11.0371 18.0274 11.0223C17.9931 11.0076 17.9564 11 17.9193 11C17.8822 11 17.8455 11.0076 17.8113 11.0223C17.777 11.0371 17.7459 11.0587 17.7197 11.086L13.6372 15.5552L11.2971 13.1275C11.2435 13.0719 11.1708 13.0407 11.0949 13.0407C11.0191 13.0407 10.9463 13.0719 10.8927 13.1275L10.0837 13.9671C10.0301 14.0227 10 14.0981 10 14.1768C10 14.2554 10.0301 14.3308 10.0837 14.3865L13.4119 17.9178C13.4426 17.949 13.48 17.9724 13.5209 17.9861C13.5619 17.9997 13.6054 18.0033 13.6479 17.9965C13.6917 18.0044 13.7367 18.0014 13.7791 17.9877C13.8216 17.974 13.8603 17.9501 13.892 17.9178L18.9173 12.3281C18.9703 12.2732 19.0001 12.1987 19.0001 12.121C19.0001 12.0434 18.9703 11.9689 18.9173 11.9139V11.914Z"
                        fill="#95A8FF"
                      />
                    </svg>
                    <span className="ml-4 text-gray-500 font-light">
                      All options
                    </span>
                  </div>
                </div>

                <a
                  className="inline-block w-full sm:w-auto px-7 py-4 text-center font-medium bg-indigo-500 hover:bg-indigo-600 text-white rounded transition duration-250"
                  href="#"
                >
                  Discover more
                </a>
              </div>
            </div>
          </div>
          <img
            className="block md:hidden w-full pl-12"
            src="/fsdemo.png"
            alt=""
          />
        </div>
      </section>

      <section className="py-12 md:py-40 bg-gray-100 relative">
        <div className="absolute left-1/6 h-32 top-0 w-px bg-gray-400"></div>
        <div className="absolute right-1/6 h-50 bottom-1/4 w-px bg-gray-400"></div>
        <div className="absolute left-1/4 h-48 bottom-0 w-px bg-gray-400"></div>

        <div className="container px-4 mx-auto relative z-10">
          {/* <div className="flex items-center mb-10">
            <span className="font-heading text-xl">01</span>
            <div className="mx-4 rounded-full bg-gray-200 h-1 w-1"></div>
            <span className="font-heading text-xl">Features</span>
          </div> */}
          <h1 className="text-center font-heading text-4xl sm:text-6xl md:text-7xl mb-24">
            Powerful features to help you manage flight strips like a pro
          </h1>

          <div className="flex flex-wrap justify-center -mx-4">
            <div className="w-full md:w-1/3 xl:w-auto px-4 mb-8 md:mb-0">
              <div className="h-full max-w-xs mx-auto p-12 bg-white rounded-xl transition-all ease-in-out hover:shadow-lg">
                <div className="mb-7">
                  <Package size={34} className="text-blue-500" />
                  <h5 className="font-heading text-xl mt-7 mb-7">
                    Feature One
                  </h5>
                  <p className="font-light">
                    Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed
                    euismod, nunc ut.
                  </p>
                </div>
                <div className="text-right">
                  <a className="inline-block" href="#">
                    <svg
                      width="16"
                      height="16"
                      viewBox="0 0 16 16"
                      fill="none"
                      xmlns="http://www.w3.org/2000/svg"
                    >
                      <path
                        d="M14.9983 2.97487L12.8444 2.94712L12.9539 11.4487L1.76433 0.259107L0.261729 1.76171L11.4513 12.9513L2.94974 12.8418L2.97749 14.9957L15.1552 15.1525L14.9983 2.97487Z"
                        fill="black"
                      ></path>
                    </svg>
                  </a>
                </div>
              </div>
            </div>
            <div className="w-full md:w-1/3 xl:w-auto px-4 mb-8 md:mb-0">
              <div className="h-full max-w-xs mx-auto p-12 bg-white rounded-xl transition-all ease-in-out hover:shadow-lg">
                <div className="mb-7">
                  <Package size={34} className="text-blue-500" />
                  <h5 className="font-heading text-xl mt-7 mb-7">
                    Feature Two
                  </h5>
                  <p className="font-light">
                    Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed
                    euismod, nunc ut.
                  </p>
                </div>
                <div className="text-right">
                  <a className="inline-block" href="#">
                    <svg
                      width="16"
                      height="16"
                      viewBox="0 0 16 16"
                      fill="none"
                      xmlns="http://www.w3.org/2000/svg"
                    >
                      <path
                        d="M14.9983 2.97487L12.8444 2.94712L12.9539 11.4487L1.76433 0.259107L0.261729 1.76171L11.4513 12.9513L2.94974 12.8418L2.97749 14.9957L15.1552 15.1525L14.9983 2.97487Z"
                        fill="black"
                      ></path>
                    </svg>
                  </a>
                </div>
              </div>
            </div>
            <div className="w-full md:w-1/3 xl:w-auto px-4 mb-8 md:mb-0">
              <div className="h-full max-w-xs mx-auto p-12 bg-white rounded-xl transition-all ease-in-out hover:shadow-lg">
                <div className="mb-7">
                  <Package size={34} className="text-blue-500" />
                  <h5 className="font-heading text-xl mt-7 mb-7">
                    Feature Three
                  </h5>
                  <p className="font-light">
                    Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed
                    euismod, nunc ut.
                  </p>
                </div>
                <div className="text-right">
                  <a className="inline-block" href="#">
                    <svg
                      width="16"
                      height="16"
                      viewBox="0 0 16 16"
                      fill="none"
                      xmlns="http://www.w3.org/2000/svg"
                    >
                      <path
                        d="M14.9983 2.97487L12.8444 2.94712L12.9539 11.4487L1.76433 0.259107L0.261729 1.76171L11.4513 12.9513L2.94974 12.8418L2.97749 14.9957L15.1552 15.1525L14.9983 2.97487Z"
                        fill="black"
                      ></path>
                    </svg>
                  </a>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>
    </main>
  );
}
