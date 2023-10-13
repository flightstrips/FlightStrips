using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public class StripUpsertRequest
{
    
    public required StripId Id { get; set; }
    public string? Origin { get; set; }
    public string? Destination { get; set; }
    public StripState State { get; set; } = StripState.None;
    public bool Cleared { get; set; }
}