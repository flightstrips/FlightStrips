using System.Collections.Concurrent;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public class ControllerService(IServiceProvider serviceProvider) : IControllerService
{
    private readonly ConcurrentDictionary<string, OnlinePosition> _onlinePositions = new(StringComparer.Ordinal);

    public async Task AddController(string connectionId, SubscribeModel subscribeModel)
    {
        var position = new OnlinePosition
        {
            Id = new OnlinePositionId(subscribeModel.Airport, subscribeModel.Session, subscribeModel.Callsign),
            PrimaryFrequency = subscribeModel.Frequency
        };
        var added = _onlinePositions.TryAdd(connectionId, position);

        if (!added) return;

        await using var scope = serviceProvider.CreateAsyncScope();
        var onlinePositionService = scope.ServiceProvider.GetRequiredService<IOnlinePositionService>();

        await onlinePositionService.CreateAsync(position.Id, position.PrimaryFrequency);
    }

    public async Task RemoveControllerAsync(string connectionId)
    {
        if (!_onlinePositions.Remove(connectionId, out var position))
        {
            return;
        }

        await using var scope = serviceProvider.CreateAsyncScope();
        var onlinePositionService = scope.ServiceProvider.GetRequiredService<IOnlinePositionService>();

        await onlinePositionService.DeleteAsync(position.Id);
    }
}
