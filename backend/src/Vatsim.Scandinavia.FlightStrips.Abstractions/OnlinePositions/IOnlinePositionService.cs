namespace Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

public interface IOnlinePositionService
{
    Task CreateAsync(string controllerId, string frequency);
    Task DeleteAsync(string controllerId);
    Task<OnlinePosition[]> ListAsync();
}
