import { useState, useEffect } from 'react'

function ZuluTime() {
  const [time, setTime] = useState(new Date())

  useEffect(() => {
    const interval = setInterval(() => {
      setTime(new Date())
    }, 1000)

    return () => clearInterval(interval)
  }, [])

  return (
    <p className="text-xl">
      {time.getUTCHours().toString().padStart(2, '0')}:
      {time.getUTCMinutes().toString().padStart(2, '0')}:
      {time.getUTCSeconds().toString().padStart(2, '0')}z
    </p>
  )
}
export default ZuluTime
