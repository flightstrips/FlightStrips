using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

public class CoordinationEntity: IAirportAndSessionTenant
{
    public int Id { get; set; }

    public CoordinationState State { get; set; }

    [MaxLength(7)]
    public required string Callsign { get; set; }

    [MaxLength(7)]
    public required string FromFrequency { get; set; }
    [MaxLength(7)]
    public required string ToFrequency { get; set; }

    [ForeignKey( $"{nameof(Callsign)},{nameof(Airport)},{nameof(Session)}")]
    public StripEntity Strip { get; set; } = null!;

    [MaxLength(4)]
    public required string Airport { get; set; } = string.Empty;
    [MaxLength(32)]
    public required string Session { get; set; } = string.Empty;
}
