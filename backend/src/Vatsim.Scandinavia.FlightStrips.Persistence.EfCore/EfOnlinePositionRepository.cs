using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Sectors;
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
            PositionFrequency = request.Frequency,
            Sector = Sector.NONE,
            FromPlugin = request.Plugin,
            ConnectedWithUi = request.Ui,
            ArrivalRunway = request.ArrivalRunway,
            DepartureRunway = request.DepartureRunway
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
                PrimaryFrequency = entity.PositionFrequency,
                Sector = entity.Sector,
                ArrivalRunway = entity.ArrivalRunway,
                DepartureRunway = entity.DepartureRunway
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

    public Task<OnlinePosition[]> ListAsync(string airport, string session, bool onlyEuroscopeConnected = false)
    {
        var controllers = _context.OnlinePositions
            .Where(x => x.Airport == airport && x.Session == session);

        if (onlyEuroscopeConnected)
        {
            controllers = controllers.Where(x => x.FromPlugin);
        }

        return controllers
            .Select(x => new OnlinePosition
            {
                Id = new OnlinePositionId(x.Airport, x.Session, x.PositionName),
                PrimaryFrequency = x.PositionFrequency,
                Sector = x.Sector
            })
            .ToArrayAsync();
    }

    public async Task BulkSetSectorAsync(SessionId id, IEnumerable<OnlinePosition> positions)
    {
        var entities = await _context.OnlinePositions.Where(x => x.Airport == id.Airport && x.Session == id.Session)
            .ToArrayAsync();

        foreach (var onlinePosition in positions)
        {
            var entity = entities.FirstOrDefault(x => x.PositionName == onlinePosition.Id.Position);
            if (entity is null) continue;

            entity.Sector = onlinePosition.Sector;
        }

        await _context.SaveChangesAsync();
    }

    public Task SetRunwaysAsync(OnlinePositionId id, string? departure, string? arrival)
    {
        return _context.OnlinePositions.Where(x => x.Airport == id.Airport && x.Session == id.Session && x.PositionName == id.Position)
            .ExecuteUpdateAsync(x =>
                x.SetProperty(o => o.DepartureRunway, departure).SetProperty(o => o.ArrivalRunway, arrival));
    }

    public Task SetUiOnlineAsync(OnlinePositionId id, bool online)
    {
        return _context.OnlinePositions.Where(x =>
                x.Airport == id.Airport && x.Session == id.Session && x.PositionName == id.Position)
            .ExecuteUpdateAsync(x => x.SetProperty(o => o.ConnectedWithUi, online));
    }
}
