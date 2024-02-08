namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;

public interface IRunwayRepository
{
    Task SetRunwayConfiguration(SessionId id, RunwayConfig config);
    Task DeleteRunwayConfig(SessionId id);
    Task<RunwayConfig?> GetRunwayConfig(SessionId id);
}
