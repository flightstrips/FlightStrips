using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;

public interface IBayService
{
    Task<bool> UpsertAsync(UpsertBayRequest request);
    Task DeleteAsync(BayId id); 
}