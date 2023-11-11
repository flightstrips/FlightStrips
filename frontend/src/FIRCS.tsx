import { Fragment, useState } from 'react'
import { Listbox, Transition } from '@headlessui/react'
import {
  CheckIcon,
  ChevronUpDownIcon,
  ClipboardDocumentCheckIcon,
  XCircleIcon,
  CheckCircleIcon,
} from '@heroicons/react/20/solid'

const atc = [
  {
    id: 1,
    name: 'EKCH - Clearance Delivery',
    short_name: 'EKCH_DEL',
    icon: <CheckCircleIcon />,
    href: '/ekch/del',
  },
  {
    id: 2,
    name: 'EKCH - Apron East',
    short_name: 'EKCH_A_GND',
    icon: <XCircleIcon />,
    href: '/ekch/gnd',
  },
  {
    id: 3,
    name: 'EKCH - Apron West',
    short_name: 'EKCH_D_GND',
    icon: <CheckCircleIcon />,
    href: '/ekch/gnd',
  },
  {
    id: 4,
    name: 'EKCH - Tower East',
    short_name: 'EKCH_A_TWR',
    icon: <CheckCircleIcon />,
    href: '/ekch/twr',
  },
  {
    id: 5,
    name: 'EKCH - Tower West',
    short_name: 'EKCH_D_TWR',
    icon: <CheckCircleIcon />,
    href: '/ekch/twr',
  },
  {
    id: 6,
    name: 'EKCH - Tower Crossing',
    short_name: 'EKCH_C_TWR',
    icon: <CheckCircleIcon />,
    href: '/ekch/ctwr',
  },
  {
    id: 7,
    name: 'EKCH - Apron Sequencing',
    short_name: 'EKCH_S_GND',
    icon: <CheckCircleIcon />,
    href: '/ekch/del',
  },
]

function classNames(...classes: any) {
  return classes.filter(Boolean).join(' ')
}

export default function FIRCS() {
  const [selected, setSelected] = useState(atc[0])

  return (
    <div className='w-screen h-screen bg-slate-900 bg-[url("https://i.imgur.com/KafT5Nx.png")] bg-cover'>
      <div className="flex p-10 justify-center items-center flex-col w-screen h-screen">
        <Listbox value={selected} onChange={setSelected}>
          {({ open }) => (
            <>
              <Listbox.Label className="block text-xl font-medium leading-6 text-white">
                Select desired station
              </Listbox.Label>
              <div className="relative mt-2 w-48">
                <Listbox.Button className="relative w-full cursor-default rounded-md bg-white py-1.5 pl-3 pr-10 text-left text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 focus:outline-none focus:ring-2 focus:ring-indigo-500 sm:text-sm sm:leading-6">
                  <span className="flex items-center">
                    <div className="h-5 w-5 flex-shrink-0 rounded-full">
                      {selected.icon}
                    </div>
                    <span className="ml-3 block truncate">
                      {selected.short_name}
                    </span>
                  </span>
                  <span className="pointer-events-none absolute inset-y-0 right-0 ml-3 flex items-center pr-2">
                    <ChevronUpDownIcon
                      className="h-5 w-5 text-gray-400"
                      aria-hidden="true"
                    />
                  </span>
                </Listbox.Button>

                <Transition
                  show={open}
                  as={Fragment}
                  leave="transition ease-in duration-100"
                  leaveFrom="opacity-100"
                  leaveTo="opacity-0"
                >
                  <Listbox.Options className="absolute z-10 mt-1 max-h-56 w-full overflow-auto rounded-md bg-white py-1 text-base shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none sm:text-sm">
                    {atc.map((atc) => (
                      <Listbox.Option
                        key={atc.id}
                        className={({ active }) =>
                          classNames(
                            active
                              ? 'bg-indigo-600 text-white'
                              : 'text-gray-900',
                            'relative cursor-default select-none py-2 pl-3 pr-9',
                          )
                        }
                        value={atc}
                      >
                        {({ selected, active }) => (
                          <a href={atc.href}>
                            <div className="flex items-center">
                              <div className="h-5 w-5 flex-shrink-0 rounded-full">
                                {atc.icon}
                              </div>
                              <span
                                className={classNames(
                                  selected ? 'font-semibold' : 'font-normal',
                                  'ml-3 block truncate',
                                )}
                              >
                                {atc.short_name}
                              </span>
                            </div>

                            {selected ? (
                              <span
                                className={classNames(
                                  active ? 'text-white' : 'text-indigo-600',
                                  'absolute inset-y-0 right-0 flex items-center pr-4',
                                )}
                              >
                                <CheckIcon
                                  className="h-5 w-5"
                                  aria-hidden="true"
                                />
                              </span>
                            ) : null}
                          </a>
                        )}
                      </Listbox.Option>
                    ))}
                  </Listbox.Options>
                </Transition>
              </div>
            </>
          )}
        </Listbox>
      </div>
    </div>
  )
}
