using System.Collections.Concurrent;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public class ControllerService(/*IServiceProvider serviceProvider*/) : IControllerService
{
    private readonly ConcurrentDictionary<string, OnlinePosition> _onlinePositions = new(StringComparer.Ordinal);

    public Task AddController(string connectionId, SubscribeModel subscribeModel, string frequency)
    {
        var position = new OnlinePosition
        {
            Id = new OnlinePositionId(subscribeModel.Airport, subscribeModel.Session, subscribeModel.Callsign),
            PrimaryFrequency = frequency
        };
        var added = _onlinePositions.TryAdd(connectionId, position);

        return Task.CompletedTask;
    }

    public Task RemoveControllerAsync(string connectionId)
    {
        _onlinePositions.Remove(connectionId, out var position);
        return Task.CompletedTask;
    }
}
