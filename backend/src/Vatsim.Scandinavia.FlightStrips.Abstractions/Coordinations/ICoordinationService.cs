namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;

public interface ICoordinationService
{
    Task<Coordination[]> ListForFrequencyAsync(SessionId session, string frequency);
    Task<Coordination?> GetForCallsignAsync(SessionId session, string callsign);
    Task<Coordination?> GetAsync(CoordinationId id);
    Task AcceptAsync(CoordinationId id, string frequency);
    Task RejectAsync(CoordinationId id, string frequency);
    Task<int> CreateAsync(Coordination coordination);
}
