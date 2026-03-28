import { createContext, useContext } from 'react';

export const UserRatingContext = createContext<number>(0);

export const useUserRating = () => useContext(UserRatingContext);
