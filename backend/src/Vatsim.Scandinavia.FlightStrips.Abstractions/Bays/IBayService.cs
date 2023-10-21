namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

public interface IBayService
{
    Task<bool> UpsertAsync(UpsertBayRequest request);
    Task DeleteAsync(string name);
    Task<Bay?> GetAsync(string name);
    Task<Bay[]> ListAsync();
    Task<string?> GetDefault(string callsign);
}
