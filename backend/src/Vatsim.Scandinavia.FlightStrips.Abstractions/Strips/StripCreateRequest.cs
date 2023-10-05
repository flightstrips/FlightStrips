using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public class StripCreateRequest
{
    
    public required StripId Id { get; set; }
    public string? Origin { get; set; }
    public string? Destination { get; set; }
    public int? Sequence { get; set; }
    public StripState State { get; set; } = StripState.None;
    public bool Cleared { get; set; }
    public string? Controller { get; set; }
    public string? NextController { get; set; }
    
}