using System.ComponentModel.DataAnnotations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;

public class Coordination
{
    [Required]
    public int Id { get; set; }

    [Required]
    public CoordinationState State { get; set; } = CoordinationState.Transfer;

    [Required]
    public required StripId StripId { get; set; }

    [Required]
    public required string FromFrequency { get; set; }

    [Required]
    public required string ToFrequency { get; set; }
}
