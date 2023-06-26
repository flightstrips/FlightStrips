import { Outlet } from 'react-router-dom'
import './Selection.css'

function Selection() {
  return (
    <>
        <form action="/ekch/del">
            <h3>EKCH - Kastrup Airport</h3>
            <select id="stations" name="stations">
                <option value="EKCH_DEL">Clearance Delivery</option>
                <option value="EKCH_A_GND">Apron East</option>
                <option value="EKCH_D_GND">Apron West</option>
                <option value="EKCH_A_TWR">Tower East</option>
                <option value="EKCH_D_TWR">Tower West</option>
                <option value="EKCH_C_TWR">Tower Crossing</option>
            </select>
            <button>Select</button>
        </form>
        <Outlet />
    </>
  )
}

export default Selection