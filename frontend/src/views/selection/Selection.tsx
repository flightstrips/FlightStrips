import { Outlet } from 'react-router-dom'
import './Selection.css'

function Selection() {
  return (
    <>
            <h3>EKCH - Kastrup Airport</h3>
            <button><a href="/ekch/del">EKCH_DEL</a></button>
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