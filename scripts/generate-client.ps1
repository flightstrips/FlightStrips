$path = "$PSScriptRoot/../frontend/src/services/api/generated"
$client = "FlightStripsClient.ts"

npx swagger-typescript-api -p http://localhost:5233/swagger/v1/swagger.json -o $path -n $client

Push-Location "$PSScriptRoot/../frontend"
npm run format
Pop-Location