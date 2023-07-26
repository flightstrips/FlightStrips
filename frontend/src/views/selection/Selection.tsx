import { Outlet } from 'react-router-dom'

function Selection() {
  return (
    <>
      <p className='text-3xl font-bold underline'>EKCH - Kastrup Airport</p>
      <button>
        <a href="/ekch/del">EKCH_DEL</a>
      </button>
      <button>EKCH_A_GND</button>
      <button>EKCH_D_GND</button>
      <button>EKCH_A_TWR</button>
      <button>EKCH_D_TWR</button>
      <button>EKCH_C_TWR</button>
      <Outlet />
    </>
  )
}

export default Selection
