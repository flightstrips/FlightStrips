using Vatsim.Scandinavia.FlightStrips.Abstractions;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public class TenantService : ITenantService
{
    private string? _airport;
    private string? _session;

    public string Airport => _airport ?? throw new InvalidOperationException("Airport has not been set");

    public string Session => _session ?? throw new InvalidOperationException("Session has not been set");

    public void SetAirport(string airport) => _airport = airport;

    public void SetSession(string session) => _session = session;
}
