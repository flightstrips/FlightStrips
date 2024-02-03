using Microsoft.AspNetCore.SignalR;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs;

public class EventHub(IControllerService controllerService, ILogger<EventHub> logger) : Hub<IEventClient>
{
    public override async Task OnDisconnectedAsync(Exception? exception)
    {
        await controllerService.RemoveControllerAsync(Context.ConnectionId);
        logger.ConnectionRemove(Context.ConnectionId);
    }

    [HubMethodName("SubscribeAirport")]
    public async Task SubscribeAirportAsync(SubscribeAirportModel request)
    {
        await Groups.AddToGroupAsync(Context.ConnectionId, ToAirportGroupName(request));
        logger.ConnectionSubscribedToAirport(Context.ConnectionId, request.Airport, request.Session);
    }

    [HubMethodName("Subscribe")]
    public async Task SubscribeAsync(SubscribeModel request)
    {
        await Groups.AddToGroupAsync(Context.ConnectionId, ToAirportGroupName(request));
        await Groups.AddToGroupAsync(Context.ConnectionId, ToFrequencyGroupName(request));
        await controllerService.AddController(Context.ConnectionId, request);
        logger.ControllerSubscribed(request.Callsign, request.Frequency, request.Airport, request.Session);
    }

    [HubMethodName("Unsubscribe")]
    public async Task UnsubscribeAsync(UnsubscribeModel request)
    {
        if (request.UnsubscribeFromAirport)
        {
            await Groups.RemoveFromGroupAsync(Context.ConnectionId, ToAirportGroupName(request));
        }
        await Groups.RemoveFromGroupAsync(Context.ConnectionId, ToFrequencyGroupName(request));
        await controllerService.RemoveControllerAsync(Context.ConnectionId);
        logger.ControllerUnsubscribed(request.Frequency, request.Airport, request.Session);
    }

    private static string ToAirportGroupName(SubscribeAirportModel model)
    {
        return $"{model.Session.ToUpperInvariant()}:{model.Airport.ToUpperInvariant()}";
    }

    private static string ToAirportGroupName(UnsubscribeModel model)
    {
        return $"{model.Session.ToUpperInvariant()}:{model.Airport.ToUpperInvariant()}";
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
