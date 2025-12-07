import { useEffect, useState } from "react"

export default function GetMetar({ icao }: { icao: string }) {
    const [metar, setMetar] = useState<string>("")

    useEffect(() => {
        fetch(`https://metar.vatsim.net/${icao}`)
            .then((res) => res.text())
            .then((data) => setMetar(data.trim()))
            .catch(() => setMetar(""))
    }, [icao])

    return metar
}
