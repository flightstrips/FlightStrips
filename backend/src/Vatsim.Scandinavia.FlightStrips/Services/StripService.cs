using Microsoft.Extensions.Logging;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Stands;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips.Events;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class StripService : IStripService
{
    private readonly IStripRepository _stripRepository;
    private readonly IStandService _standService;
    private readonly IBayService _bayService;
    private readonly IEventService _eventService;
    private readonly ILogger<StripService> _logger;

    public StripService(IStripRepository stripRepository, ILogger<StripService> logger, IBayService bayService, IEventService eventService, IStandService standService)
    {
        _stripRepository = stripRepository;
        _logger = logger;
        _bayService = bayService;
        _eventService = eventService;
        _standService = standService;
    }

    public async Task HandleStripUpdateAsync(FullStripEvent stripEvent)
    {
        var existing = await _stripRepository.GetAsync(stripEvent.Id);

        if (existing is not null)
        {
            Map(existing, stripEvent);
            await _stripRepository.UpdateAsync(existing);
            return;
        }

        var isDeparture = stripEvent.Id.Airport.Equals(stripEvent.Origin, StringComparison.OrdinalIgnoreCase);

        var bay = await _bayService.GetDefaultAsync(stripEvent.Id.Airport, stripEvent.Id.Callsign, isDeparture);

        if (string.IsNullOrEmpty(bay))
        {
            throw new InvalidOperationException("Must have a bay");
        }

        var strip = new Strip {Id = stripEvent.Id, Bay = bay };

        Map(strip, stripEvent);
        await _stripRepository.CreateAsync(strip);
    }

    public async Task HandleStripPositionUpdateAsync(PositionEvent positionEvent)
    {
        var stand = await _standService.GetStandAsync(positionEvent.Id.Airport, positionEvent.Position.Location);
        if (stand is null) return;


        // TODO send to flow control system.

        await _eventService.SendPositionUpdate(positionEvent.Id, positionEvent.Position);

        if (await _stripRepository.SetStandAsync(positionEvent.Id, stand.Name))
        {
            await SendUpdateEvent(positionEvent.Id);
        }
    }

    public async Task SetSquawkAsync(StripId id, string squawk)
    {
        if (await _stripRepository.SetSquawk(id, squawk))
        {
            await SendUpdateEvent(id);
        }
    }

    public async Task SetAssignedSquawkAsync(StripId id, string squawk)
    {
        if (await _stripRepository.SetAssignedSquawkAsync(id, squawk))
        {
            await SendUpdateEvent(id);
        }
    }

    public async Task SetFinalAltitudeAsync(StripId id, int altitude)
    {
        if (await _stripRepository.SetFinalAltitudeAsync(id, altitude))
        {
            await SendUpdateEvent(id);
        }
    }

    public async Task SetClearedAltitudeAsync(StripId id, int altitude)
    {
        if (await _stripRepository.SetClearedAltitudeAsync(id, altitude))
        {
            await SendUpdateEvent(id);
        }
    }

    public async Task SetCommunicationTypeAsync(StripId id, CommunicationType communicationType)
    {
        if (await _stripRepository.SetCommunicationTypeAsync(id, communicationType))
        {
            await SendUpdateEvent(id);
        }
    }

    public async Task SetGroundStateAsync(StripId id, StripState state)
    {
        if (await _stripRepository.SetGroundStateAsync(id, state))
        {
            await SendUpdateEvent(id);
        }
    }

    private static void Map(Strip strip, FullStripEvent stripEvent)
    {
        strip.Destination = stripEvent.Destination;
        strip.Origin = stripEvent.Origin;
        strip.Sequence = null;
        strip.Capabilities = stripEvent.Capabilities.ToString();
        strip.Cleared = stripEvent.Cleared;
        strip.Alternate = stripEvent.Alternate;
        strip.Route = stripEvent.Route;
        strip.Heading = stripEvent.Heading;
        strip.Remarks = stripEvent.Remarks;
        strip.Runway = stripEvent.Runway;
        strip.Sid = stripEvent.Sid;
        strip.AssignedSquawk = stripEvent.AssignedSquawk;
        strip.State = stripEvent.State;
        strip.AircraftCategory = stripEvent.AircraftCategory;
        strip.AircraftType = stripEvent.AircraftType;
        strip.ClearedAltitude = stripEvent.ClearedAltitude;
        strip.CommunicationType = stripEvent.CommunicationType;
        strip.FinalAltitude = stripEvent.FinalAltitude;
        strip.TOBT = stripEvent.TOBT;
    }

    public async Task<(bool created, Strip strip)> UpsertStripAsync(StripUpsertRequest upsertRequest)
    {
        var strip = await GetStripAsync(upsertRequest.Id);

        if (string.IsNullOrEmpty(strip?.Bay))
        {
            var isDeparture = upsertRequest.Id.Airport.Equals(upsertRequest.Origin, StringComparison.OrdinalIgnoreCase);

            var bay = await _bayService.GetDefaultAsync(upsertRequest.Id.Airport, upsertRequest.Id.Callsign, isDeparture);

            if (string.IsNullOrEmpty(bay))
            {
                throw new InvalidOperationException("Must have a bay");

            }

            upsertRequest.Bay = bay;
        }

        var (created, result) = await _stripRepository.UpsertAsync(upsertRequest);

        await _eventService.StripUpdatedAsync(result);

        return (created, result);
    }

    public Task<Strip[]> ListAsync(SessionId id)
    {
        return _stripRepository.ListAsync(id);
    }

    public async Task DeleteStripAsync(StripId id)
    {
        var strip = await _stripRepository.GetAsync(id);

        if (strip is null)
        {
            return;
        }

        await _stripRepository.DeleteAsync(id);
        await _eventService.StripDeletedAsync(strip);
    }

    public Task<Strip?> GetStripAsync(StripId id)
    {
        return _stripRepository.GetAsync(id);
    }

    public async Task SetSequenceAsync(StripId id, int? sequence)
    {
        _logger.SetSequence(id, sequence);

        await _stripRepository.SetSequenceAsync(id, sequence);
        await SendUpdateEvent(id);

    }

    public async Task SetBayAsync(StripId id, string bayName)
    {
        await _stripRepository.SetBayAsync(id, bayName);
        await SendUpdateEvent(id);
    }

    public async Task AssumeAsync(StripId id, string frequency)
    {
        await _stripRepository.SetPositionFrequencyAsync(id, frequency);
        await SendUpdateEvent(id);
    }

    public Task<SessionId[]> GetSessionsAsync() => _stripRepository.GetSessionsAsync();

    public Task RemoveSessionAsync(SessionId id) => _stripRepository.RemoveSessionAsync(id);

    public async Task ClearAsync(StripId id, bool isCleared)
    {
        var bay = "STARTUP";
        if (!isCleared)
        {
            bay = await _bayService.GetDefaultAsync(id.Airport, id.Callsign, true) ?? "OTHER";
        }

        await _stripRepository.SetCleared(id, isCleared, bay);
    }

    private async Task SendUpdateEvent(StripId id)
    {
        var strip = await _stripRepository.GetAsync(id);

        if (strip is null) return;

        await _eventService.StripUpdatedAsync(strip);
    }
}
