import { observer } from "mobx-react";
import Flightstrip from "../data/interfaces/flightstrip";
import Strip from "./strip";

export const StripList = observer(({ strips }: {strips: Flightstrip[]}) => {
    return (
        <>
            {strips.map(plan => {
                return <Strip plan={plan} />
            })}
        </>)
})