using FlightStrips;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public class EuroScopeService(IEuroScopeClients clients) : IEuroScopeService
{
    public Task SetClearedAsync(StripId id, string controller, bool isCleared)
    {
        var session = new SessionId(id.Airport, id.Session);

        var serverMessage = new ServerStreamMessage
        {
            StripUpdate = new StripResponse { Callsign = id.Callsign, Cleared = new ClearedFlag {Cleared = isCleared}}
        };

        return clients.WriteToControllerClientAsync(session, controller, serverMessage);
    }
}
