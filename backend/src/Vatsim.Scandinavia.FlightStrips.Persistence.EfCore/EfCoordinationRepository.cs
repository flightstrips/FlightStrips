using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
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
            Airport = coordination.StripId.Airport,
            Session = coordination.StripId.Session,
            Callsign = coordination.StripId.Callsign,
            FromFrequency = coordination.FromFrequency,
            ToFrequency = coordination.ToFrequency,
            State = coordination.State
        };

        _context.Coordination.Add(entity);

        await _context.SaveChangesAsync();

        return entity.Id;
    }

    public Task DeleteAsync(CoordinationId id)
    {
        return _context.Coordination.Where(x => x.Airport == id.Airport && x.Session == id.Session && x.Id == id.Id)
            .ExecuteDeleteAsync();
    }

    public Task<Coordination[]> ListForFrequency(SessionId session, string frequency)
    {
        return _context.Coordination.Where(x =>
                x.Airport == session.Airport && x.Session == session.Session &&
                (x.FromFrequency == frequency || x.ToFrequency == frequency))
            .Select(x => new Coordination
            {
                Id = x.Id,
                StripId = new StripId(x.Airport, x.Session, x.Callsign),
                FromFrequency = x.FromFrequency,
                ToFrequency = x.ToFrequency,
                State = x.State
            }).ToArrayAsync();
    }

    public async Task<Coordination?> GetAsync(CoordinationId id)
    {
        var entity = await _context.Coordination.FirstOrDefaultAsync(x =>
            x.Airport == id.Airport && x.Session == id.Session && x.Id == id.Id);

        return entity is null
            ? null
            : new Coordination
            {
                StripId = new StripId(entity.Airport, entity.Session, entity.Callsign),
                FromFrequency = entity.FromFrequency,
                ToFrequency = entity.ToFrequency,
                State = entity.State
            };
    }

    public async Task<Coordination?> GetForCallsignAsync(SessionId session, string callsign)
    {
        var entity = await _context.Coordination.FirstOrDefaultAsync(x =>
            x.Airport == session.Airport && x.Session == session.Session && x.Callsign == callsign);

        return entity is null
            ? null
            : new Coordination
            {
                StripId = new StripId(entity.Airport, entity.Session, entity.Callsign),
                FromFrequency = entity.FromFrequency,
                ToFrequency = entity.ToFrequency,
                State = entity.State
            };

    }
}
