using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public interface IOwnerService
{
    OnlinePosition[] GetOwners(SessionId sessionId, RunwayConfig? runwayConfig, OnlinePosition[] onlinePositions);
}
