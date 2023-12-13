import { Api } from './api/generated/FlightStripsClient.ts'

const client = new Api({ baseUrl: 'http://localhost:5233' })

export default client
