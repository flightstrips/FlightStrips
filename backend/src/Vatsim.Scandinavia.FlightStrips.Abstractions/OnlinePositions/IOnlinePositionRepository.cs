namespace Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

public interface IOnlinePositionRepository
{
    Task AddAsync(OnlinePositionAddRequest request);
    Task DeleteAsync(string positionName);
    Task<OnlinePosition[]> ListAsync();
}
