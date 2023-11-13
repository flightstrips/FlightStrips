using System.ComponentModel.DataAnnotations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public class Strip
{
    [Required]
    public required string Callsign { get; set; }
    public string? Origin { get; set; }
    public string? Destination { get; set; }
    public int? Sequence { get; set; }
    public StripState State { get; set; }
    public bool Cleared { get; set; }

    public string? PositionFrequency { get; set; }

    [Required]
    public string Bay { get; set; } = string.Empty;

    [Required]
    public DateTime LastUpdated { get; set; }
}
