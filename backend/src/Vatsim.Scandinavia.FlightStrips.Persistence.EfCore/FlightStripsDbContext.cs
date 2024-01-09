using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public class FlightStripsDbContext : DbContext
{
    public DbSet<StripEntity> Strips { get; set; } = null!;
    public DbSet<OnlinePositionEntity> OnlinePositions { get; set; } = null!;

    public DbSet<CoordinationEntity> Coordination { get; set; } = null!;

    public FlightStripsDbContext(DbContextOptions<FlightStripsDbContext> options) : base(options)
    {
    }
}
