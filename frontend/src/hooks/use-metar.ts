import { useCallback, useEffect, useState } from "react"

export function useMetar(icao: string): { metar: string | null; refetch: () => void } {
    const [metar, setMetar] = useState<string | null>(null)

    const fetchMetar = useCallback(() => {
        if (!icao) return
        fetch(`https://metar.vatsim.net/${icao}`)
            .then((res) => res.text())
            .then((data) => setMetar(data.trim()))
            .catch(() => setMetar(null))
    }, [icao])

    useEffect(() => {
        if (!icao) return
        fetchMetar()
        const interval = setInterval(fetchMetar, 2 * 60 * 1000)
        return () => clearInterval(interval)
    }, [icao, fetchMetar])

    return { metar, refetch: fetchMetar }
}