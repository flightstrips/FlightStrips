using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

public class OnlinePosition
{
    [Required]
    public required string PositionId { get; set; }

    [Required]
    public required string PrimaryFrequency { get; set; }
}
