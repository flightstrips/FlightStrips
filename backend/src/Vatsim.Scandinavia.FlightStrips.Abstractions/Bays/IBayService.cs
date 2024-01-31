namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

public interface IBayService
{
    Task<Bay?> GetAsync(string airport, string name);
    Task<Bay[]> ListAsync(string airport);
    Task<string?> GetDefaultAsync(string airport, string callsign, bool isDeparture);
}
