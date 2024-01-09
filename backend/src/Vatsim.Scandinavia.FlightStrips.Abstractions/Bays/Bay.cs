using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

public class Bay
{
    [Required]
    public required string Name { get; set; }

    [Required]
    public BayDefaultType Default { get; set; }

    [Required]
    public string[] CallsignFilter { get; set; } = Array.Empty<string>();
}

public enum BayDefaultType
{
    Arrival,
    Departure,
    None
}
