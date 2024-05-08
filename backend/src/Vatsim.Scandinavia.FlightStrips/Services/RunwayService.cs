using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class RunwayService(IRunwayRepository runwayRepository, IOnlinePositionService onlinePositionService, IEventService eventService) : IRunwayService
{
    public async Task SetRunwaysAsync(SessionId id, RunwayConfig config)
    {
        await runwayRepository.SetRunwayConfiguration(id, config);
        await eventService.SendRunwayConfigurationUpdate(id, config);
        await onlinePositionService.UpdateSectorsAsync(id);
    }

    public async Task DeleteRunwaysAsync(SessionId id)
    {
        await runwayRepository.DeleteRunwayConfig(id);
        await onlinePositionService.UpdateSectorsAsync(id);
    }

    public Task<RunwayConfig?> GetRunwayConfigAsync(SessionId id)
    {
        return runwayRepository.GetRunwayConfig(id);
    }

}
