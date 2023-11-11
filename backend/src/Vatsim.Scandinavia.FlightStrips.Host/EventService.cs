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

public class EventService : IEventService
{
    private readonly IHubContext<EventHub, IEventClient> _hubContext;
    private readonly ITenantService _tenantService;

    public EventService(ITenantService tenantService, IHubContext<EventHub, IEventClient> hubContext)
    {
        _tenantService = tenantService;
        _hubContext = hubContext;
    }

    public Task ControllerOnlineAsync(OnlinePosition position) => ControllerUpdateAsync(position, true);

    public Task ControllerOfflineAsync(OnlinePosition position) => ControllerUpdateAsync(position, false);

    private Task ControllerUpdateAsync(OnlinePosition position, bool online)
    {
        var model = new ControllerUpdateModel
        {
            Frequency = position.PrimaryFrequency,
            Position = position.PositionId,
            State = online ? ControllerState.Online : ControllerState.Offline
        };

        return _hubContext.Clients.Group(ToAirportAndSessionGroup()).ReceiveControllerUpdate(model);
    }

    public Task StripCreatedAsync(Strip strip) => StripUpdateAsync(strip, StripState.Created);
    public Task StripUpdatedAsync(Strip strip) => StripUpdateAsync(strip, StripState.Updated);
    public Task StripDeletedAsync(Strip strip) => StripUpdateAsync(strip, StripState.Deleted);

    private Task StripUpdateAsync(Strip strip, StripState status)
    {
        var model = new StripUpdateModel
        {
            Callsign = strip.Callsign,
            State = strip.State,
            Bay = strip.Bay,
            Cleared = strip.Cleared,
            Destination = strip.Destination,
            Origin = strip.Origin,
            Sequence = strip.Sequence,
            PositionFrequency = strip.PositionFrequency
        };

        return _hubContext.Clients.Group(ToAirportAndSessionGroup()).ReceiveStripUpdate(model);
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
            Callsign = coordination.Callsign,
            CoordinationId = coordination.Id
        };

        return _hubContext.Clients.Group(ToAirportAndSessionGroup()).ReceiveCoordinationUpdate(model);
    }

    private string ToAirportAndSessionGroup() =>
        ToAirportAndSessionGroup(_tenantService.Airport, _tenantService.Session);
    private static string ToAirportAndSessionGroup(string airport, string session) => $"{session}:{airport}";
}
