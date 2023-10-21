using Microsoft.EntityFrameworkCore;

using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;
using Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public class EfBayRepository : IBayRepository
{
    private readonly FlightStripsDbContext _context;

    public EfBayRepository(FlightStripsDbContext context)
    {
        _context = context;
    }

    public async Task<bool> UpsertAsync(UpsertBayRequest request)
    {
        var entity = await _context.Bays
                .FirstOrDefaultAsync(x => x.Name == request.Id);

        var created = entity is null;

        if (entity is null)
        {
            entity = new BayEntity {Name = request.Id,};
            _context.Bays.Add(entity);
        }

        entity.Default = request.Default;
        entity.Filters = request.CallsignFilter.Select(x => new BayFilter{ Callsign = x }).ToList();

        await _context.SaveChangesAsync();

        return created;
    }

    public Task DeleteAsync(string name)
    {
        return _context.Bays.Where(x => x.Name == name).ExecuteDeleteAsync();
    }

    public async Task<Bay?> GetAsync(string name)
    {
        var entity = await _context.Bays.FirstOrDefaultAsync(x => x.Name == name);

        if (entity is null)
        {
            return null;
        }

        return new Bay
        {
            Name = entity.Name,
            Default = entity.Default,
            CallsignFilter = entity.Filters.Select(x => x.Callsign).ToArray()
        };
    }

    public async Task<Bay[]> ListAsync(ListBaysRequest request)
    {
        return await _context.Bays
            .Where(x => !request.Default.HasValue || x.Default == request.Default.Value)
            .Select(x => new Bay
            {
                Name = x.Name,
                Default = x.Default,
                CallsignFilter = x.Filters.Select(f => f.Callsign).ToArray()
            }).ToArrayAsync();
    }
}
