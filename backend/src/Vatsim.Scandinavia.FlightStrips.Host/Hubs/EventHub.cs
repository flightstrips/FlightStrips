using Microsoft.AspNetCore.SignalR;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs;

public class EventHub(IControllerService controllerService, IOnlinePositionService onlinePositionService, ILogger<EventHub> logger) : Hub<IEventClient>
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

        if (request.IncludePositionUpdates)
        {
            await Groups.AddToGroupAsync(Context.ConnectionId, ToPositionUpdateGroupName(request));
        }

        logger.ConnectionSubscribedToAirport(Context.ConnectionId, request.Airport, request.Session);
    }

    [HubMethodName("Subscribe")]
    public async Task<string> SubscribeAsync(SubscribeModel request)
    {
        var positionId = new OnlinePositionId(request.Airport, request.Session, request.Callsign);
        var position = await onlinePositionService.GetAsync(positionId);

        if (position is null)
        {
            // There is no position online with this callsign
            throw new InvalidOperationException($"No position online with callsign {request.Callsign}");
        }

        await onlinePositionService.SetUiOnlineAsync(positionId, online: true);
        await Groups.AddToGroupAsync(Context.ConnectionId, ToAirportGroupName(request));
        await Groups.AddToGroupAsync(Context.ConnectionId, ToFrequencyGroupName(request, position.PrimaryFrequency));
        await controllerService.AddController(Context.ConnectionId, request, position.PrimaryFrequency);
        logger.ControllerSubscribed(request.Callsign, request.Airport, request.Session);

        return position.PrimaryFrequency;
    }

    [HubMethodName("Unsubscribe")]
    public async Task UnsubscribeAsync(UnsubscribeModel request)
    {
        if (request.UnsubscribeFromAirport)
        {
            await Groups.RemoveFromGroupAsync(Context.ConnectionId, ToAirportGroupName(request));
            await Groups.RemoveFromGroupAsync(Context.ConnectionId, ToPositionUpdateGroupName(request));
        }

        var positionId = new OnlinePositionId(request.Airport, request.Session, request.Callsign);
        var position = await onlinePositionService.GetAsync(positionId);

        if (position is not null)
        {
            await Groups.RemoveFromGroupAsync(Context.ConnectionId, ToFrequencyGroupName(request, position.PrimaryFrequency));
        }

        await onlinePositionService.SetUiOnlineAsync(positionId, online: true);
        await controllerService.RemoveControllerAsync(Context.ConnectionId);
        logger.ControllerUnsubscribed(request.Airport, request.Session);
    }

    private static string ToAirportGroupName(SubscribeAirportModel model)
    {
        return $"{model.Session.ToUpperInvariant()}:{model.Airport.ToUpperInvariant()}";
    }

    private static string ToPositionUpdateGroupName(SubscribeAirportModel model)
    {
        return $"{ToAirportGroupName(model)}:position";
    }

    private static string ToAirportGroupName(UnsubscribeModel model)
    {
        return $"{model.Session.ToUpperInvariant()}:{model.Airport.ToUpperInvariant()}";
    }

    private static string ToPositionUpdateGroupName(UnsubscribeModel model)
    {
        return $"{ToAirportGroupName(model)}:position";
    }

    private static string ToFrequencyGroupName(SubscribeModel model, string frequency)
    {
        return $"{ToAirportGroupName(model)}:{frequency}";
    }

    private static string ToFrequencyGroupName(UnsubscribeModel model, string frequency)
    {
        return $"{ToAirportGroupName(model)}:{frequency}";
    }
}
