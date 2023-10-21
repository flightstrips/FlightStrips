using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public class EfOnlinePositionRepository
{
    private readonly FlightStripsDbContext _context;

    public EfOnlinePositionRepository(FlightStripsDbContext context)
    {
        _context = context;
    }

    public async Task AddAsync(OnlinePositionAddRequest request)
    {
        var position = await _context.Positions.AsNoTracking()
            .FirstOrDefaultAsync(x => x.Frequency == request.Frequency);

        if (position is null)
        {
            throw new InvalidOperationException($"Unknown position frequency {request.Frequency}");
        }

        var entity = new OnlinePositionEntity {PositionName = request.Name, PositionId = position.Id};

        _context.OnlinePositions.Add(entity);

        await _context.SaveChangesAsync();
    }

    // TODO support update

    public Task DeleteAsync(string positionName)
    {
        return _context.OnlinePositions.Where(x => x.PositionName == positionName).ExecuteDeleteAsync();
    }

    public Task<OnlinePosition[]> ListAsync()
    {
        return _context.OnlinePositions
            .Select(x => new OnlinePosition {PositionId = x.PositionName, PrimaryFrequency = x.Position.Frequency})
            .ToArrayAsync();
    }
}
