using System.Linq.Expressions;

using Microsoft.EntityFrameworkCore;

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
        var entity = await _context.Strips.FirstOrDefaultAsync(x => x.Callsign == upsertRequest.Callsign);
        var bay = await _context.Bays.FirstOrDefaultAsync(x => x.Name == upsertRequest.Bay);

        if (bay is null)
        {
            throw new InvalidOperationException("Bay does not exist");
        }

        var created = entity is null;

        if (entity is null)
        {
            entity = new StripEntity
            {
                Callsign = upsertRequest.Callsign
            };
            _context.Add(entity);
        }

        entity.BayId = bay.Id;
        entity.Destination = upsertRequest.Destination;
        entity.Origin = upsertRequest.Origin;
        entity.State = upsertRequest.State;

        await _context.SaveChangesAsync();

        return created;

    }

    public Task DeleteAsync(string callsign)
    {
        return _context.Strips
            .Where(x => x.Callsign == callsign)
            .ExecuteDeleteAsync();
    }


    public async Task<Strip?> GetAsync(string callsign)
    {
        var entity = await _context.Strips.Include(x => x.Bay).FirstOrDefaultAsync(x => x.Callsign == callsign);

        if (entity is null)
        {
            return null;
        }

        return new Strip
        {
            Callsign = entity.Callsign,
            Destination = entity.Destination,
            Origin = entity.Origin,
            State = entity.State,
            Cleared = entity.Cleared,
            Sequence = entity.Sequence,
            Bay = entity.Bay.Name,
        };
    }

    public async Task SetSequenceAsync(string callsign, int? sequence)
    {
        var current = await _context.Strips.Where(x => x.Callsign == callsign).Select(x => new {x.Sequence})
            .FirstOrDefaultAsync();

        if (current is null)
        {
            throw new InvalidOperationException($"Strip with callsign {callsign} not found.");
        }

        if (current.Sequence == sequence)
        {
            return;
        }

        if (current.Sequence is null && sequence.HasValue)
        {
            await _context.Strips.Where(x => x.Sequence >= sequence).ExecuteUpdateAsync(x =>
                x.SetProperty(entity => entity.Sequence, entity => entity.Sequence + 1));
        }

        if (current.Sequence < sequence)
        {
            await _context.Strips.Where(x => x.Sequence > current.Sequence && x.Sequence <= sequence)
                .ExecuteUpdateAsync(x => x.SetProperty(entity => entity.Sequence, entity => entity.Sequence - 1));
        }

        if (current.Sequence > sequence)
        {
            await _context.Strips.Where(x => x.Sequence <= current.Sequence && x.Sequence > sequence)
                .ExecuteUpdateAsync(x => x.SetProperty(entity => entity.Sequence, entity => entity.Sequence + 1));
        }

        await _context.Strips.Where(x => x.Callsign == callsign)
            .ExecuteUpdateAsync(x => x.SetProperty(entity => entity.Sequence, sequence));
    }
}
