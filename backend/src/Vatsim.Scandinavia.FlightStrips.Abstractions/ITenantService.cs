namespace Vatsim.Scandinavia.FlightStrips.Abstractions;

public interface ITenantService
{
    string Airport { get; }

    string Session { get; }

    void SetAirport(string airport);
    void SetSession(string session);
}
