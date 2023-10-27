namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Positions;

public interface IPositionService
{
    Task UpsertAsync(UpsertPositionRequest request);
    Task DeleteAsync(string frequency);
    Task<Position?> GetAsync(string frequency);
    Task<Position[]> ListAsync();
}