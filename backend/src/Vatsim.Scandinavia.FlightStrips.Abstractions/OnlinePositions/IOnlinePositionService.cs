namespace Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

public interface IOnlinePositionService
{
    Task CreateAsync(OnlinePositionId id, string frequency);
    Task DeleteAsync(OnlinePositionId id);
    Task<OnlinePosition[]> ListAsync(string airport, string session);
}
