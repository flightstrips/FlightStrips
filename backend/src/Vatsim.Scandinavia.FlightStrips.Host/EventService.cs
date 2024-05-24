using Microsoft.AspNetCore.SignalR;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;
using CoordinationState = Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models.CoordinationState;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public class EventService(
    IHubContext<EventHub, IEventClient> hubContext, ILogger<EventService> logger)
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

    public Task StripDeletedAsync(Strip strip)
    {
        var model = new StripDisconnectedModel {Callsign = strip.Id.Callsign};

        var group = ToAirportAndSessionGroup(strip.Id);

        return hubContext.Clients.Group(group).ReceiveStripDeleted(model);
    }

    public Task StripUpdatedAsync(Strip strip)
    {

        var model = new StripUpdateModel
        {
            Callsign = strip.Id.Callsign,
            Bay = strip.Bay,
            Controller = strip.PositionFrequency,
            Cleared = strip.Cleared,
            Destination = strip.Destination,
            Origin = strip.Origin,
            Sequence = strip.Sequence,
            Alternate = strip.Alternate,
            Capabilities = strip.Capabilities,
            Remarks = strip.Remarks,
            Route = strip.Route,
            Runway = strip.Runway,
            Squawk = strip.Squawk,
            Stand = strip.Stand,
            Tobt = strip.TOBT,
            AircraftCategory = strip.AircraftCategory,
            AircraftType = strip.AircraftType,
            AssignedSquawk = strip.AssignedSquawk,
            CommunicationType = strip.CommunicationType,
            Heading = strip.Heading,
            Sid = strip.Sid,
            Tsat = strip.TSAT,
            ClearedAltitude = strip.ClearedAltitude,
            FinalAltitude = strip.FinalAltitude

        };
        var group = ToAirportAndSessionGroup(strip.Id);

        logger.SendingStripUpdate(strip.Id.Callsign, strip.Id.Airport, strip.Id.Session, group);

        return hubContext.Clients.Group(group).ReceiveStripUpdate(model);
    }

    public Task SendControllerSectorsAsync(SessionId id, IEnumerable<OnlinePosition> onlinePositions)
    {
        var model = onlinePositions.DistinctBy(x => x.PrimaryFrequency).Select(x => new SectorUpdateModel
        {
            Frequency = x.PrimaryFrequency, Sectors = x.Sector.ToString().Split(", ")
        }).ToArray();

        var group = ToAirportAndSessionGroup(id);
        return hubContext.Clients.Group(group).ReceiveControllerSectorsUpdate(model);
    }

    public Task SendRunwayConfigurationUpdate(SessionId id, RunwayConfig runwayConfig)
    {
        var model = new RunwayConfigurationModel {Arrival = runwayConfig.Arrival, Departure = runwayConfig.Departure};

        var group = ToAirportAndSessionGroup(id);
        return hubContext.Clients.Group(group).ReceiveRunwayConfigurationUpdate(model);
    }

    public Task SendPositionUpdate(StripId id, Position position)
    {
        var model = new StripPositionUpdate()
        {
            Callsign = id.Callsign,
            Altitude = position.Height,
            Latitude = position.Location.Latitude,
            Longitude = position.Location.Longitude
        };

        var group = $"{ToAirportAndSessionGroup(id)}:position";

        return hubContext.Clients.Group(group).ReceiveStripPositionUpdate(model);
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
    private static string ToAirportAndSessionGroup(SessionId id) => ToAirportAndSessionGroup(id.Airport, id.Session);
    private static string ToAirportAndSessionGroup(string airport, string session) => $"{session.ToUpperInvariant()}:{airport.ToUpperInvariant()}";
}
