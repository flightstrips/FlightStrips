namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

public interface IBayRepository
{
    Task<bool> UpsertAsync(UpsertBayRequest request);
    Task DeleteAsync(BayId id);
    Task<Bay?> GetAsync(BayId id);
    Task<IEnumerable<Bay>> ListAsync(ListBaysRequest request);
}