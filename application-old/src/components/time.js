import React, { useState, useEffect } from 'react';

function ZuluTime() {
  const [time, setTime] = useState(new Date());

  useEffect(() => {
    const interval = setInterval(() => {
      setTime(new Date());
    }, 1000);

    return () => clearInterval(interval);
  }, []);

  return <p className='text-xl'>{time.getUTCHours()}:{time.getUTCMinutes()}:{time.getUTCSeconds()}z</p>;
}
export default ZuluTime;

//