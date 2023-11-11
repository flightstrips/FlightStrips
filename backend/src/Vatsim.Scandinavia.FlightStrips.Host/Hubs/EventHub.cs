using Microsoft.AspNetCore.SignalR;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs;

public class EventHub : Hub<IEventClient>
{
    [HubMethodName("Subscribe")]
    public async Task SubscribeAsync(SubscribeModel request)
    {
        await Groups.AddToGroupAsync(Context.ConnectionId, ToAirportGroupName(request));
        await Groups.AddToGroupAsync(Context.ConnectionId, ToFrequencyGroupName(request));
    }

    [HubMethodName("Unsubscribe")]
    public async Task UnsubscribeAsync(UnsubscribeModel request)
    {
        if (request.UnsubscribeFromAirport)
        {
            await Groups.RemoveFromGroupAsync(Context.ConnectionId, ToAirportGroupName(request));
        }
        await Groups.RemoveFromGroupAsync(Context.ConnectionId, ToFrequencyGroupName(request));
    }

    private static string ToAirportGroupName(SubscribeModel model)
    {
        return $"{model.Session}:{model.Airport}";
    }

    private static string ToAirportGroupName(UnsubscribeModel model)
    {
        return $"{model.Session}:{model.Airport}";
    }

    private static string ToFrequencyGroupName(SubscribeModel model)
    {
        return $"{ToAirportGroupName(model)}:{model.Frequency}";
    }

    private static string ToFrequencyGroupName(UnsubscribeModel model)
    {
        return $"{ToAirportGroupName(model)}:{model.Frequency}";
    }
}
