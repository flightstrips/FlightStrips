using System.ComponentModel.DataAnnotations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Sectors;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

public class OnlinePosition
{
    [Required]
    public required OnlinePositionId Id { get; set; }

    [Required]
    public required string PrimaryFrequency { get; set; }

    public Sector Sector { get; set; } = Sector.NONE;
}
