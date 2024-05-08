using System.Globalization;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Stands;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class StandService : IStandService
{

    private ILookup<string, Stand>? _stands;

    public async Task<Stand?> GetStandAsync(string airport, Location location)
    {
        await LoadStandsAsync();
        var stands = _stands![airport];

        Stand? closest = null;
        double min = 1000;

        foreach (var stand in stands)
        {
            var distance = stand.Location.Distance(location);
            if (distance < stand.Radius && distance < min)
            {
                min = distance;
                closest = stand;
            }
        }

        return closest;
    }


    private async Task LoadStandsAsync()
    {
        if (_stands is null)
        {
            var lines = await File.ReadAllLinesAsync(
                @"C:\Users\fsr19\AppData\Roaming\EuroScope\EKDK\Plugins\GRpluginStands.txt");

            _stands = lines.Where(x => x.StartsWith("STAND", StringComparison.Ordinal)).Select(x =>
            {
                var split = x.Split(':');
                var stand = new Stand(split[2], Location.FromCoordinateString(split[3], split[4]),
                    int.Parse(split[5], CultureInfo.InvariantCulture));
                return (split[1], stand);
            }).ToLookup(x => x.Item1, x => x.stand, StringComparer.OrdinalIgnoreCase);
        }

    }
}
