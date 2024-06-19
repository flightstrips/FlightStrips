using FlightStrips;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public interface IEuroScopeClients
{
    Task<bool> AddClientAsync(OnlinePositionId id, IEuroScopeClient client);
    Task RemoveClientAsync(OnlinePositionId id);
    Task WriteToControllerClientAsync(SessionId session, string controller, ServerStreamMessage message);
}
