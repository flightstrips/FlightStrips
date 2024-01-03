namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;

public interface ICoordinationRepository
{
    Task<int> CreateAsync(Coordination coordination);
    Task DeleteAsync(CoordinationId id);
    Task<Coordination[]> ListForFrequency(SessionId session, string frequency);
    Task<Coordination?> GetAsync(CoordinationId id);
    Task<Coordination?> GetForCallsignAsync(SessionId session, string callsign);
}
