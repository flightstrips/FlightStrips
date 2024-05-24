using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions;

public interface IEuroScopeEventService
{
    Task SetClearedAsync(StripId id, OnlinePositionId positionId);
    Task UpdateRoute(StripId id, OnlinePositionId positionId, string route);
    Task UpdateRemarks(StripId id, OnlinePositionId positionId, string route);
}

