using System.Collections.Concurrent;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Masters;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class MasterService : IMasterService
{
    private readonly ConcurrentDictionary<SessionId, string> _masters = new();


    public bool IsMaster(OnlinePositionId id)
    {
        var sessionId = new SessionId(id.Airport, id.Session);
        return _masters.TryGetValue(sessionId, out var controller) &&
               id.Position.Equals(controller, StringComparison.OrdinalIgnoreCase);
    }

    public bool SetMaster(OnlinePositionId id)
    {
        var sessionId = new SessionId(id.Airport, id.Session);
        _masters.AddOrUpdate(sessionId, _ => id.Position, (_, _) => id.Position);
        return true;
    }
}
