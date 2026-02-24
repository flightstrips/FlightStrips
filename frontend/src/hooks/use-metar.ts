import { useEffect, useState } from "react"

export function useMetar(icao: string): string {
    const [metar, setMetar] = useState<string>("")

    useEffect(() => {
        fetch(`https://metar.vatsim.net/${icao}`)
            .then((res) => res.text())
            .then((data) => setMetar(data.trim()))
            .catch(() => setMetar(""))
    }, [icao])

    return metar
}
