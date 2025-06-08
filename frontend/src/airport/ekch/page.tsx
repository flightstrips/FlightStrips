import {Link} from "react-router-dom";

export default function Airport() {
  return ( 
    <div style={{fontFamily: "Plus Jakarta Sans"}} className="bg-[#003d48] w-scren h-full min-h-screen text-white flex flex-col justify-center items-center">
        <h1 className="py-2 text-lg">Views</h1>
        <Link to="/airport/ekch/DEL">Delivery</Link>
        <Link to="/airport/ekch/AAAD">AAAD</Link>
        <Link to="/airport/ekch/GWGE">GWGE</Link>
        <Link to="/airport/ekch/TWTE">TWTE</Link>
    </div>
  ) }