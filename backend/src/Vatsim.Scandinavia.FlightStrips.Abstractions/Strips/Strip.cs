using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public class Strip
{
    public required string Callsign { get; set; }
    public string? Origin { get; set; }
    public string? Destination { get; set; }
    public int? Sequence { get; set; }
    public StripState State { get; set; }
    public bool Cleared { get; set; }

    public string? Controller { get; set; }
    public string? NextController { get; set; }

    public string BayId { get; set; } = string.Empty;
}
