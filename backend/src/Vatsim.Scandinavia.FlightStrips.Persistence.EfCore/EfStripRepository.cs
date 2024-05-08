using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public class EfStripRepository(FlightStripsDbContext context) : IStripRepository
{
    public async Task CreateAsync(Strip strip)
    {
        var entity = new StripEntity
        {
            Airport = strip.Id.Airport,
            Callsign = strip.Id.Callsign,
            Session = strip.Id.Session,
            BayName = strip.Bay,
            Destination = strip.Destination,
            Origin = strip.Origin
        };

        UpdateStrip(entity, strip);

        context.Add(entity);
        await context.SaveChangesAsync();
    }

    public async Task<bool> SetStandAsync(StripId id, string stand)
    {
        var changed = await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign)
            .ExecuteUpdateAsync(x => x.SetProperty(p => p.Stand, stand));

        return changed == 1;
    }

    public async Task<bool> SetSquawk(StripId id, string squawk)
    {
        var changed = await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign && x.Squawk != squawk)
            .ExecuteUpdateAsync(x => x.SetProperty(s => s.Squawk, squawk));

        return changed == 1;
    }

    public async Task<bool> SetAssignedSquawkAsync(StripId id, string squawk)
    {
        var changed = await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign && x.AssignedSquawk != squawk)
            .ExecuteUpdateAsync(x => x.SetProperty(s => s.AssignedSquawk, squawk));

        return changed == 1;
    }

    public async Task<bool> SetFinalAltitudeAsync(StripId id, int altitude)
    {
        var changed = await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign && x.FinalAltitude != altitude)
            .ExecuteUpdateAsync(x => x.SetProperty(s => s.FinalAltitude, altitude));

        return changed == 1;
    }

    public async Task<bool> SetClearedAltitudeAsync(StripId id, int altitude)
    {
        var changed = await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign && x.ClearedAltitude != altitude)
            .ExecuteUpdateAsync(x => x.SetProperty(s => s.ClearedAltitude, altitude));

        return changed == 1;
    }

    public async Task<bool> SetGroundStateAsync(StripId id, StripState state)
    {
        var changed = await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign && x.State != state)
            .ExecuteUpdateAsync(x => x.SetProperty(s => s.State, state));

        return changed == 1;
    }

    public async Task<bool> SetCommunicationTypeAsync(StripId id, CommunicationType communicationType)
    {
        var changed = await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign && x.CommunicationType != communicationType)
            .ExecuteUpdateAsync(x => x.SetProperty(s => s.CommunicationType, communicationType));

        return changed == 1;
    }

    public async Task UpdateAsync(Strip strip)
    {
        var entity = await context.Strips.FindAsync([strip.Id.Callsign, strip.Id.Session, strip.Id.Airport]);

        if (entity is null)
        {
            throw new InvalidOperationException("Strip does not exist");
        }

        UpdateStrip(entity, strip);

        await context.SaveChangesAsync();
    }

    public async Task<(bool, Strip)> UpsertAsync(StripUpsertRequest upsertRequest)
    {
        var id = upsertRequest.Id;
        var entity = await context.Strips.FindAsync([id.Callsign, id.Session, id.Airport]);
        var created = entity is null;

        if (entity is null)
        {
            entity = new StripEntity
            {
                Airport = id.Airport,
                Session = id.Session,
                Callsign = id.Callsign,
                Origin = string.Empty,
                Destination = string.Empty,
                BayName = upsertRequest.Bay ?? "NONE"
            };
            context.Add(entity);
        }

        entity.BayName = string.IsNullOrEmpty(upsertRequest.Bay) ? entity.BayName : upsertRequest.Bay;
        entity.Destination = upsertRequest.Destination ?? string.Empty;
        entity.Origin = upsertRequest.Origin ?? string.Empty;
        entity.State = upsertRequest.State;
        entity.Cleared = upsertRequest.Cleared;

        await context.SaveChangesAsync();

        return (created, Map(entity));
    }

    public Task DeleteAsync(StripId id)
    {
        return context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign)
            .ExecuteDeleteAsync();
    }

    public async Task<Strip?> GetAsync(StripId id)
    {
        var entity = await context.Strips.FindAsync([id.Callsign, id.Session, id.Airport]);

        if (entity is null)
        {
            return null;
        }

        return Map(entity);
    }

    public async Task SetSequenceAsync(StripId id, int? sequence)
    {
        var current = await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign)
            .Select(x => new { x.Sequence })
            .FirstOrDefaultAsync();

        if (current is null)
        {
            throw new InvalidOperationException($"Strip with id {id} not found.");
        }

        if (current.Sequence == sequence)
        {
            return;
        }

        if (current.Sequence is null && sequence.HasValue)
        {
            await context.Strips
                .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Sequence >= sequence)
                .ExecuteUpdateAsync(x =>
                    x.SetProperty(entity => entity.Sequence, entity => entity.Sequence + 1));
        }

        if (current.Sequence < sequence)
        {
            await context.Strips.Where(x =>
                    x.Airport == id.Airport && x.Session == id.Session && x.Sequence > current.Sequence &&
                    x.Sequence <= sequence)
                .ExecuteUpdateAsync(x => x.SetProperty(entity => entity.Sequence, entity => entity.Sequence - 1));
        }

        if (current.Sequence > sequence)
        {
            await context.Strips.Where(x =>
                    x.Airport == id.Airport && x.Session == id.Session && x.Sequence <= current.Sequence &&
                    x.Sequence > sequence)
                .ExecuteUpdateAsync(x => x.SetProperty(entity => entity.Sequence, entity => entity.Sequence + 1));
        }

        await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign)
            .ExecuteUpdateAsync(x => x.SetProperty(entity => entity.Sequence, sequence));
    }

    public async Task SetCleared(StripId id, bool isCleared, string bay)
    {
        var count = await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign)
            .ExecuteUpdateAsync(x =>
                x.SetProperty(strip => strip.BayName, bay).SetProperty(strip => strip.Cleared, isCleared));

        if (count != 1)
        {
            throw new InvalidOperationException("Strip does not exist");
        }

    }

    public async Task SetBayAsync(StripId id, string bayName)
    {
        var count = await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign)
            .ExecuteUpdateAsync(x => x.SetProperty(strip => strip.BayName, bayName));

        if (count != 1)
        {
            throw new InvalidOperationException("Strip does not exist");
        }
    }

    public async Task SetPositionFrequencyAsync(StripId id, string frequency)
    {
        var count = await context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign)
            .ExecuteUpdateAsync(x => x.SetProperty(strip => strip.PositionFrequency, frequency));

        if (count != 1)
        {
            throw new InvalidOperationException("Strip does not exist");
        }

    }

    public Task<SessionId[]> GetSessionsAsync()
    {
        return context.Strips.GroupBy(x => new { x.Airport, x.Session })
            .Select(x => new SessionId(x.Key.Airport, x.Key.Session))
            .ToArrayAsync();
    }

    public Task RemoveSessionAsync(SessionId id)
    {
        return context.Strips.Where(x => x.Airport == id.Airport && x.Session == id.Session)
            .ExecuteDeleteAsync();
    }

    private static Strip Map(StripEntity entity)
    {
        return new Strip
        {
            Id = new StripId(entity.Airport, entity.Session, entity.Callsign),
            Destination = entity.Destination,
            Origin = entity.Origin,
            State = entity.State,
            Cleared = entity.Cleared,
            Sequence = entity.Sequence,
            Bay = entity.BayName,
            PositionFrequency = entity.PositionFrequency,
            Alternate = entity.Alternate,
            Capabilities = entity.Capabilities,
            Heading = entity.Heading,
            Remarks = entity.Remarks,
            Runway = entity.Runway,
            Sid = entity.Sid,
            Squawk = entity.Squawk,
            Stand = entity.Stand,
            AircraftCategory = entity.AircraftCategory,
            AircraftType = entity.AircraftType,
            ClearedAltitude = entity.ClearedAltitude,
            CommunicationType = entity.CommunicationType,
            FinalAltitude = entity.FinalAltitude,
            AOBT = entity.AOBT,
            ASAT = entity.ASAT,
            CTOT = entity.CTOT,
            TOBT = entity.TOBT,
            TSAT = entity.TSAT,
            TTOT = entity.TTOT
        };
    }

    private static void UpdateStrip(StripEntity entity, Strip strip)
    {
        entity.Destination = strip.Destination;
        entity.Origin = strip.Origin;
        entity.Sequence = strip.Sequence;
        entity.Capabilities = strip.Capabilities;
        entity.Cleared = strip.Cleared;
        entity.Route = strip.Route;
        entity.Alternate = strip.Alternate;
        entity.Heading = strip.Heading;
        entity.Remarks = strip.Remarks;
        entity.Runway = strip.Runway;
        entity.Sid = strip.Sid;
        entity.AssignedSquawk = strip.AssignedSquawk;
        entity.Squawk = strip.Squawk;
        entity.Stand = strip.Stand;
        entity.State = strip.State;
        entity.AircraftCategory = strip.AircraftCategory;
        entity.AircraftType = strip.AircraftType;
        entity.BayName = strip.Bay;
        entity.ClearedAltitude = strip.ClearedAltitude;
        entity.CommunicationType = strip.CommunicationType;
        entity.FinalAltitude = strip.FinalAltitude;
        entity.PositionFrequency = strip.PositionFrequency;
        entity.TTOT = strip.TTOT;
        entity.AOBT = strip.AOBT;
        entity.TOBT = strip.TOBT;
        entity.TSAT = strip.TSAT;
        entity.ASAT = strip.ASAT;
        entity.CTOT = strip.CTOT;
    }

}
