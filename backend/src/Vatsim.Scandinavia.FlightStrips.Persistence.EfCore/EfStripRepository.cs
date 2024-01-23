using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public class EfStripRepository : IStripRepository
{
    private readonly FlightStripsDbContext _context;

    public EfStripRepository(FlightStripsDbContext context)
    {
        _context = context;
    }

    public async Task<bool> UpsertAsync(StripUpsertRequest upsertRequest)
    {
        var id = upsertRequest.Id;
        var entity = await _context.Strips.FirstOrDefaultAsync(x =>
            x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign);
        var created = entity is null;

        if (entity is null)
        {
            entity = new StripEntity
            {
                Airport = id.Airport,
                Session = id.Session,
                Callsign = id.Callsign,
                BayName = upsertRequest.Bay!
            };
            _context.Add(entity);
        }

        entity.BayName = upsertRequest.Bay!;
        entity.Destination = upsertRequest.Destination;
        entity.Origin = upsertRequest.Origin;
        entity.State = upsertRequest.State;

        await _context.SaveChangesAsync();

        return created;

    }

    public Task DeleteAsync(StripId id)
    {
        return _context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign)
            .ExecuteDeleteAsync();
    }

    public async Task<Strip?> GetAsync(StripId id)
    {
        var entity = await _context.Strips.FirstOrDefaultAsync(x =>
            x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign);

        if (entity is null)
        {
            return null;
        }

        return new Strip
        {
            Id = new StripId(entity.Airport, entity.Session, entity.Callsign),
            Destination = entity.Destination,
            Origin = entity.Origin,
            State = entity.State,
            Cleared = entity.Cleared,
            Sequence = entity.Sequence,
            Bay = entity.BayName,
            LastUpdated = entity.UpdatedTime,
            PositionFrequency = entity.PositionFrequency
        };
    }

    public async Task SetSequenceAsync(StripId id, int? sequence)
    {
        var current = await _context.Strips
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
            await _context.Strips
                .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Sequence >= sequence)
                .ExecuteUpdateAsync(x =>
                    x.SetProperty(entity => entity.Sequence, entity => entity.Sequence + 1));
        }

        if (current.Sequence < sequence)
        {
            await _context.Strips.Where(x =>
                    x.Airport == id.Airport && x.Session == id.Session && x.Sequence > current.Sequence &&
                    x.Sequence <= sequence)
                .ExecuteUpdateAsync(x => x.SetProperty(entity => entity.Sequence, entity => entity.Sequence - 1));
        }

        if (current.Sequence > sequence)
        {
            await _context.Strips.Where(x =>
                    x.Airport == id.Airport && x.Session == id.Session && x.Sequence <= current.Sequence &&
                    x.Sequence > sequence)
                .ExecuteUpdateAsync(x => x.SetProperty(entity => entity.Sequence, entity => entity.Sequence + 1));
        }

        await _context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign)
            .ExecuteUpdateAsync(x => x.SetProperty(entity => entity.Sequence, sequence));
    }

    public async Task SetBayAsync(StripId id, string bayName)
    {
        var count = await _context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign)
            .ExecuteUpdateAsync(x => x.SetProperty(strip => strip.BayName, bayName));

        if (count != 1)
        {
            throw new InvalidOperationException("Strip does not exist");
        }
    }

    public async Task SetPositionFrequencyAsync(StripId id, string frequency)
    {
        var count = await _context.Strips
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Callsign == id.Callsign)
            .ExecuteUpdateAsync(x => x.SetProperty(strip => strip.PositionFrequency, frequency));

        if (count != 1)
        {
            throw new InvalidOperationException("Strip does not exist");
        }

    }

    public Task<SessionId[]> GetSessionsAsync()
    {
        return _context.Strips.GroupBy(x => new { x.Airport, x.Session })
            .Select(x => new SessionId(x.Key.Airport, x.Key.Session))
            .ToArrayAsync();
    }

    public Task RemoveSessionAsync(SessionId id)
    {
        return _context.Strips.Where(x => x.Airport == id.Airport && x.Session == id.Session)
            .ExecuteDeleteAsync();
    }

}
