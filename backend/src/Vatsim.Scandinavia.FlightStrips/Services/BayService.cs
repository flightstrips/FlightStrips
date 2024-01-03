using Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class BayService : IBayService
{
    private static readonly Bay[] s_bays =
    [
        new Bay { Name = "OTHER", Default = BayDefaultType.Arrival },
        new Bay { Name = "SAS", Default = BayDefaultType.Arrival, CallsignFilter = ["SAS"] },
        new Bay { Name = "NORWEGIAN", Default = BayDefaultType.Arrival, CallsignFilter = ["IBK", "NZS", "NAX"] },
        new Bay { Name = "STARTUP", Default = BayDefaultType.None },
        new Bay { Name = "PUSHBACK", Default = BayDefaultType.None },
        new Bay { Name = "TWY ARR", Default = BayDefaultType.None },
        new Bay { Name = "TWY DEP", Default = BayDefaultType.None },
        new Bay { Name = "DE-ICE", Default = BayDefaultType.None },
        new Bay { Name = "RWY ARR", Default = BayDefaultType.None },
        new Bay { Name = "RWY DEP", Default = BayDefaultType.None },
        new Bay { Name = "AIRBORNE", Default = BayDefaultType.None },
        new Bay { Name = "STAND", Default = BayDefaultType.None },
        new Bay { Name = "FINAL", Default = BayDefaultType.Arrival },
    ];

    public Task<Bay?> GetAsync(string airport, string name)
    {
        if (airport.Equals("EKCH", StringComparison.OrdinalIgnoreCase))
        {
            return Task.FromResult<Bay?>(null);
        }

        return Task.FromResult(s_bays.FirstOrDefault(x => x.Name.Equals(name, StringComparison.OrdinalIgnoreCase)));
    }

    public Task<Bay[]> ListAsync(string airport)
    {
        if (airport.Equals("EKCH", StringComparison.OrdinalIgnoreCase))
        {
            return Task.FromResult(Array.Empty<Bay>());
        }

        return Task.FromResult(s_bays);
    }

    public Task<string?> GetDefault(string airport, string callsign, bool isDeparture)
    {
        if (airport.Equals("EKCH", StringComparison.OrdinalIgnoreCase))
        {
            return Task.FromResult<string?>(null);
        }

        var company = callsign.Trim()[..3];

        var defaultBays = s_bays
            .Where(x => x.Default == (isDeparture ? BayDefaultType.Departure : BayDefaultType.Arrival)).ToArray();

        if (defaultBays.Length == 0)
        {
            return Task.FromResult<string?>(null);
        }

        var bay = defaultBays.FirstOrDefault(x =>
                      x.CallsignFilter.Length != 0 &&
                      x.CallsignFilter.Contains(company, StringComparer.OrdinalIgnoreCase)) ??
                  defaultBays.FirstOrDefault(x => x.CallsignFilter.Length == 0);

        return Task.FromResult(bay?.Name);
    }
}
