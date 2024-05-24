using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions;

public interface IEventService
{
    Task ControllerOnlineAsync(OnlinePosition position);
    Task ControllerOfflineAsync(OnlinePosition position);
    Task AtisUpdateAsync();
    Task AcceptCoordinationAsync(Coordination coordination);
    Task RejectCoordinationAsync(Coordination coordination);
    Task StartCoordinationAsync(Coordination coordination);
    Task StripUpdatedAsync(Strip strip);
    Task StripDeletedAsync(Strip strip);
    Task SendControllerSectorsAsync(SessionId id, IEnumerable<OnlinePosition> onlinePositions);
    Task SendRunwayConfigurationUpdate(SessionId id, RunwayConfig runwayConfig);
    Task SendPositionUpdate(StripId id, Position position);
}
