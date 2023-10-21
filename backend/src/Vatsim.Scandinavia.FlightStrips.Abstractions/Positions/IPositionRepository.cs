namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Positions;

public interface IPositionRepository
{
    Task<bool> UpsertAsync(UpsertPositionRequest request);
    Task DeleteAsync(string frequency);
    Task<Position?> GetAsync(string frequency);
    Task<Position[]> ListAsync();
}
