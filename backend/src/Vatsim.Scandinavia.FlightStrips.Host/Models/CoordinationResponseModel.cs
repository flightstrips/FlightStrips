using System.ComponentModel.DataAnnotations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class CoordinationResponseModel
{
    [Required]
    public int Id { get; init; }

    [Required]
    public CoordinationState State { get; init; } = CoordinationState.Transfer;

    [Required]
    public required string Callsign { get; init; }

    [Required]
    public required string FromFrequency { get; init; }

    [Required]
    public required string ToFrequency { get; init; }
}
