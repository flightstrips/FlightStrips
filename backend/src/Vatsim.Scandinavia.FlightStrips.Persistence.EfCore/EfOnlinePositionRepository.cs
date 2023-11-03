using Microsoft.EntityFrameworkCore;
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
        var entity = new OnlinePositionEntity {PositionName = request.Name, PositionFrequency = request.Frequency};

        _context.OnlinePositions.Add(entity);

        await _context.SaveChangesAsync();
    }

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
