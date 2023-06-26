import axios from 'axios';

function ATIS() {
  const getAtis = () =>  {
    axios.get('https://data.vatsim.net/v3/vatsim-data.json')
    .then(response => {
      console.log(response)
    })
    .catch(err => {
      console.log(err)
    })
  }
    return (
        <>
          {getAtis}
        </>
    )
  }

export default  ATIS;