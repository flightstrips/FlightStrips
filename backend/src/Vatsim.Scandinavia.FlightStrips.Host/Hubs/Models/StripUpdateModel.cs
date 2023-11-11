namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

public class StripUpdateModel
{

    public required string Callsign { get; set; }
    public string? Origin { get; set; }
    public string? Destination { get; set; }
    public int? Sequence { get; set; }
    public Abstractions.Enums.StripState State { get; set; }
    public bool Cleared { get; set; }

    public string? PositionFrequency { get; set; }

    public string Bay { get; set; } = string.Empty;
}
