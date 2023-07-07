import BayHeader from './BayHeader'
import { observer } from 'mobx-react'
import { StripList } from './StripList'
import { useFlightStripStore } from '../providers/RootStoreContext'

export const BasePlate = observer(() => {
  const flightStripStore = useFlightStripStore()

  return (
    <div className="baseplate">
      <div className="baseBay">
        <BayHeader
          name="others"
          showNewButton={true}
          showPlannedButton={true}
        />
        <StripList strips={flightStripStore.pending(false)} />
      </div>
      <div className="baseBay">
        <BayHeader name="SAS" />
        <StripList strips={flightStripStore.pending(true)} />
      </div>
      <div className="baseBay">
        <BayHeader name="CLEARED" />
        <StripList strips={flightStripStore.cleared()} />
      </div>
      <div className="baseBay">d</div>
    </div>
  )
})
