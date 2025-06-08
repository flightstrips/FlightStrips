"use client";

import React, { useState, useEffect } from 'react';

const CurrentUTC: React.FC = () => {
    const [date, setDate] = useState(new Date());

    useEffect(() => {
        const interval = setInterval(() => {
            setDate(new Date());
        }, 1000);

        return () => clearInterval(interval);
    }, []);

    const formattedDate = `${date.getUTCHours().toString().padStart(2, '0')}:${date.getUTCMinutes().toString().padStart(2, '0')}:${date.getUTCSeconds().toString().padStart(2, '0')}Z`;
    return <div className='w-full h-full flex justify-center items-center'>{formattedDate}</div>;
};

export { CurrentUTC };