using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public class EfOnlinePositionRepository : IOnlinePositionRepository
{
    private readonly FlightStripsDbContext _context;

    public EfOnlinePositionRepository(FlightStripsDbContext context)
    {
        _context = context;
    }

    public async Task AddAsync(OnlinePositionAddRequest request)
    {
        var entity = new OnlinePositionEntity
        {
            Airport = request.Id.Airport,
            Session = request.Id.Session,
            PositionName = request.Id.Position,
            PositionFrequency = request.Frequency
        };

        _context.OnlinePositions.Add(entity);

        await _context.SaveChangesAsync();
    }

    public Task DeleteAsync(OnlinePositionId id)
    {
        return _context.OnlinePositions
            .Where(x => x.Airport == id.Airport && x.Session == id.Session && x.PositionName == id.Position)
            .ExecuteDeleteAsync();
    }

    public async Task<OnlinePosition?> GetAsync(OnlinePositionId id)
    {
        var entity = await _context.OnlinePositions.FirstOrDefaultAsync(x =>
            x.Airport == id.Airport && x.Session == id.Session && x.PositionName == id.Position);
        return entity is null
            ? null
            : new OnlinePosition
            {
                Id = new OnlinePositionId(entity.Airport, entity.Session, entity.PositionName),
                PrimaryFrequency = entity.PositionFrequency
            };
    }

    public Task<SessionId[]> GetSessionsAsync()
    {
        return _context.OnlinePositions.GroupBy(x => new { x.Airport, x.Session })
            .Select(x => new SessionId(x.Key.Airport, x.Key.Session))
            .ToArrayAsync();
    }

    public Task RemoveSessionAsync(SessionId id)
    {
        return _context.OnlinePositions.Where(x => x.Airport == id.Airport && x.Session == id.Session)
            .ExecuteDeleteAsync();
    }

    public Task<OnlinePosition[]> ListAsync(string airport, string session)
    {
        return _context.OnlinePositions
            .Where(x => x.Airport == airport && x.Session == session)
            .Select(x => new OnlinePosition
            {
                Id = new OnlinePositionId(x.Airport, x.Session, x.PositionName),
                PrimaryFrequency = x.PositionFrequency
            })
            .ToArrayAsync();
    }
}
