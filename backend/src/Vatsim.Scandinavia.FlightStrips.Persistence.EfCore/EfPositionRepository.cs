using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Positions;
using Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public class EfPositionRepository : IPositionRepository
{
    private readonly FlightStripsDbContext _context;

    public EfPositionRepository(FlightStripsDbContext context)
    {
        _context = context;
    }

    public async Task<bool> UpsertAsync(UpsertPositionRequest request)
    {
        var entity = await _context.Positions.FirstOrDefaultAsync(x => x.Frequency == request.Frequency);

        var created = entity is null;

        if (entity is null)
        {
            entity = new PositionEntity {Frequency = request.Frequency, Name = request.Name};
            _context.Positions.Add(entity);
        }

        entity.Name = request.Name;

        await _context.SaveChangesAsync();

        return created;
    }

    public Task DeleteAsync(string frequency)
    {
        return _context.Positions.Where(x => x.Frequency == frequency).ExecuteDeleteAsync();
    }

    public async Task<Position?> GetAsync(string frequency)
    {
        var entity = await _context.Positions.FirstOrDefaultAsync(x => x.Frequency == frequency);

        if (entity is null)
        {
            return null;
        }

        var position = new Position {Frequency = entity.Frequency, Name = entity.Name};

        return position;
    }

    public async Task<Position[]> ListAsync()
    {
        return await _context.Positions.Select(x => new Position {Frequency = x.Frequency, Name = x.Name})
            .ToArrayAsync();
    }
}
