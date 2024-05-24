namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;

public static class RunwayHelper
{
    public static (string? arrival, string? departure) GetRunways(ActiveRunway[] runways)
    {
        var departure = runways.FirstOrDefault(x =>
                            x.IsDeparture && (x.Runway.StartsWith("22", StringComparison.OrdinalIgnoreCase) ||
                                              x.Runway.StartsWith("04", StringComparison.OrdinalIgnoreCase))) ??
                        runways.FirstOrDefault(x => x.IsDeparture);

        var arrival = runways.FirstOrDefault(x =>
                            !x.IsDeparture && (x.Runway.StartsWith("22", StringComparison.OrdinalIgnoreCase) ||
                                              x.Runway.StartsWith("04", StringComparison.OrdinalIgnoreCase))) ??
                        runways.FirstOrDefault(x => !x.IsDeparture);

        return (arrival?.Runway, departure?.Runway);
    }

}
