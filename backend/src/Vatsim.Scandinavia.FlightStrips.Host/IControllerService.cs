using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public interface IControllerService
{
    Task AddController(string connectionId, SubscribeModel subscribeModel, string frequency);
    Task RemoveControllerAsync(string connectionId);
}
