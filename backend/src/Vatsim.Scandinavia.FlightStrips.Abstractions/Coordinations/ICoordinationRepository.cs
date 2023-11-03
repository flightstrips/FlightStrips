namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;

public interface ICoordinationRepository
{
    Task<int> CreateAsync(Coordination coordination);
    Task DeleteAsync(int id);
    Task<Coordination[]> ListForFrequency(string frequency);
    Task<Coordination?> GetAsync(int id);
    Task<Coordination?> GetForCallsignAsync(string callsign);
}
