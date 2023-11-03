using System.ComponentModel.DataAnnotations.Schema;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

public class CoordinationEntity: IAirportAndSessionTenant
{
    public int Id { get; set; }

    public CoordinationState State { get; set; }

    public required string Callsign { get; set; }

    public required string FromFrequency { get; set; }
    public required string ToFrequency { get; set; }

    [ForeignKey( $"{nameof(FromFrequency)},{nameof(Airport)}")]
    public PositionEntity From { get; set; } = null!;

    [ForeignKey( $"{nameof(ToFrequency)},{nameof(Airport)}")]
    public PositionEntity To { get; set; } = null!;

    [ForeignKey( $"{nameof(Callsign)},{nameof(Airport)},{nameof(Session)}")]
    public StripEntity Strip { get; set; } = null!;

    public string Airport { get; set; } = string.Empty;
    public string Session { get; set; } = string.Empty;
}
