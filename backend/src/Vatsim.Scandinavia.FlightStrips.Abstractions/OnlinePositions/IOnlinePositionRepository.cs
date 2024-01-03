namespace Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

public interface IOnlinePositionRepository
{
    Task AddAsync(OnlinePositionAddRequest request);
    Task DeleteAsync(OnlinePositionId id);
    Task<OnlinePosition[]> ListAsync(string airport, string session);
    Task<OnlinePosition?> GetAsync(OnlinePositionId id);
}
