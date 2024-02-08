namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;

public interface IRunwayService
{
    Task SetRunwaysAsync(SessionId id, RunwayConfig config);
    Task DeleteRunwaysAsync(SessionId id);
    Task<RunwayConfig?> GetRunwayConfigAsync(SessionId id);
}
