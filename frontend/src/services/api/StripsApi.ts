import { Api } from './generated/FlightStripsClient.ts'

const client = new Api({ baseUrl: 'http://localhost:5233' })

export default client
