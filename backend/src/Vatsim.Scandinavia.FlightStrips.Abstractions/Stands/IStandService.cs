using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Stands;

public interface IStandService
{

    /// <summary>
    /// Get the stand which corresponds to the location. If the location is not within any stands <c>null</c> is returned.
    /// </summary>
    public Task<Stand?> GetStandAsync(string airport, Location location);
}
