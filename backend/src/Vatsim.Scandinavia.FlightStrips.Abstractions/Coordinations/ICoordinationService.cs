namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;

public interface ICoordinationService
{
    Task<Coordination[]> ListForFrequencyAsync(string frequency);
    Task<Coordination?> GetForCallsignAsync(string callsign);
    Task<Coordination?> GetAsync(int id);
    Task AcceptAsync(int id, string frequency);
    Task RejectAsync(int id, string frequency);
    Task<int> CreateAsync(Coordination coordination);
}
