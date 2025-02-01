import Link from "next/link";

export default function Airport() {
  return ( 
    <div style={{fontFamily: "Plus Jakarta Sans"}} className="bg-[#003d48] w-scren h-full min-h-screen text-white flex flex-col justify-center items-center">
        <h1 className="py-2 text-lg">Views</h1>
        <Link href="/airport/ekch/DEL">Delivery</Link>
        <Link href="/airport/ekch/AAAD">AAAD</Link>
        <Link href="/airport/ekch/GWGE">GWGE</Link>
        <Link href="/airport/ekch/TWTE">TWTE</Link>
    </div>
  ) }