using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;
using Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public class EfRunwayRepository(FlightStripsDbContext context) : IRunwayRepository
{
    public async Task SetRunwayConfiguration(SessionId id, RunwayConfig config)
    {
        var entity = await context.RunwayConfigs.FindAsync([id.Session, id.Airport]);

        if (entity is null)
        {
            entity = new RunwayConfigurationEntity {Airport = id.Airport, Session = id.Session};
            context.Add(entity);
        }

        entity.Departure = config.Departure;
        entity.Arrival = config.Arrival;
        entity.Position = config.Position;

        await context.SaveChangesAsync();
    }

    public Task DeleteRunwayConfig(SessionId id)
    {
        return context.RunwayConfigs.Where(x => x.Session == id.Session && x.Airport == id.Airport)
            .ExecuteDeleteAsync();
    }

    public async Task<RunwayConfig?> GetRunwayConfig(SessionId id)
    {
        var entity = await context.RunwayConfigs.FindAsync([id.Session, id.Airport]);

        return entity is null ? null : new RunwayConfig(entity.Departure, entity.Arrival, entity.Position);
    }

}
