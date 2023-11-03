using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public class EfCoordinationRepository : ICoordinationRepository
{
    private readonly FlightStripsDbContext _context;

    public EfCoordinationRepository(FlightStripsDbContext context)
    {
        _context = context;
    }

    public async Task<int> CreateAsync(Coordination coordination)
    {
        var entity = new CoordinationEntity
        {
            Callsign = coordination.Callsign,
            FromFrequency = coordination.FromFrequency,
            ToFrequency = coordination.ToFrequency,
            State = coordination.State
        };

        _context.Coordination.Add(entity);

        await _context.SaveChangesAsync();

        return entity.Id;
    }

    public Task DeleteAsync(int id)
    {
        return _context.Coordination.Where(x => x.Id == id).ExecuteDeleteAsync();
    }

    public Task<Coordination[]> ListForFrequency(string frequency)
    {
        return _context.Coordination.Where(x => x.FromFrequency == frequency || x.ToFrequency == frequency)
            .Select(x => new Coordination
            {
                Callsign = x.Callsign, FromFrequency = x.FromFrequency, ToFrequency = x.ToFrequency, State = x.State
            }).ToArrayAsync();
    }

    public async Task<Coordination?> GetAsync(int id)
    {
        var entity = await _context.Coordination.FirstOrDefaultAsync(x => x.Id == id);

        return entity is null
            ? null
            : new Coordination
            {
                Callsign = entity.Callsign,
                FromFrequency = entity.FromFrequency,
                ToFrequency = entity.ToFrequency,
                State = entity.State
            };
    }

    public async Task<Coordination?> GetForCallsignAsync(string callsign)
    {
        var entity = await _context.Coordination.FirstOrDefaultAsync(x => x.Callsign == callsign);

        return entity is null
            ? null
            : new Coordination
            {
                Callsign = entity.Callsign,
                FromFrequency = entity.FromFrequency,
                ToFrequency = entity.ToFrequency,
                State = entity.State
            };

    }
}
