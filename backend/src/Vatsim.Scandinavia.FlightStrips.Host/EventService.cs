using Microsoft.AspNetCore.SignalR;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;
using CoordinationState = Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models.CoordinationState;
using StripState = Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models.StripState;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public class EventService(
    IHubContext<EventHub, IEventClient> hubContext,
    ILogger<EventService> logger)
    : IEventService
{
    public Task ControllerOnlineAsync(OnlinePosition position) => ControllerUpdateAsync(position, true);

    public Task ControllerOfflineAsync(OnlinePosition position) => ControllerUpdateAsync(position, false);

    private Task ControllerUpdateAsync(OnlinePosition position, bool online)
    {
        var model = new ControllerUpdateModel
        {
            Frequency = position.PrimaryFrequency,
            Position = position.Id.Position,
            State = online ? ControllerState.Online : ControllerState.Offline
        };

        return hubContext.Clients.Group(ToAirportAndSessionGroup(position.Id.Airport, position.Id.Session)).ReceiveControllerUpdate(model);
    }

    public Task StripCreatedAsync(Strip strip) => StripUpdateAsync(strip, StripState.Created);
    public Task StripUpdatedAsync(Strip strip) => StripUpdateAsync(strip, StripState.Updated);
    public Task StripDeletedAsync(Strip strip) => StripUpdateAsync(strip, StripState.Deleted);

    private Task StripUpdateAsync(Strip strip, StripState status)
    {
        logger.LogInformation("Sending strip update {@Strip} ", strip);
        var model = new StripUpdateModel
        {
            Callsign = strip.Id.Callsign,
            State = strip.State,
            EventState = status,
            Bay = strip.Bay,
            Cleared = strip.Cleared,
            Destination = strip.Destination,
            Origin = strip.Origin,
            Sequence = strip.Sequence,
            PositionFrequency = strip.PositionFrequency
        };

        return hubContext.Clients.Group(ToAirportAndSessionGroup(strip.Id)).ReceiveStripUpdate(model);
    }

    public Task AtisUpdateAsync() => throw new NotImplementedException();

    public Task AcceptCoordinationAsync(Coordination coordination) =>
        CoordinationUpdateAsync(coordination, CoordinationState.Accepted);

    public Task RejectCoordinationAsync(Coordination coordination) =>
        CoordinationUpdateAsync(coordination, CoordinationState.Rejected);

    public Task StartCoordinationAsync(Coordination coordination) =>
        CoordinationUpdateAsync(coordination, CoordinationState.Created);

    private Task CoordinationUpdateAsync(Coordination coordination, CoordinationState state)
    {
        var model = new CoordinationUpdateModel
        {
            To = coordination.ToFrequency,
            From = coordination.FromFrequency,
            State = state,
            Callsign = coordination.StripId.Callsign,
            CoordinationId = coordination.Id
        };

        return hubContext.Clients.Group(ToAirportAndSessionGroup(coordination.StripId)).ReceiveCoordinationUpdate(model);
    }

    private static string ToAirportAndSessionGroup(StripId id) => ToAirportAndSessionGroup(id.Airport, id.Session);
    private static string ToAirportAndSessionGroup(string airport, string session) => $"{session}:{airport}";
}
