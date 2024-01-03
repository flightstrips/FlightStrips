using System.ComponentModel.DataAnnotations.Schema;
using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

[PrimaryKey(nameof(Callsign), nameof(Session), nameof(Airport))]
public class StripEntity : IAirportAndSessionTenant
{
    public string Session { get; set; } = string.Empty;

    public string Airport { get; set; } = string.Empty;
    public required string Callsign { get; set; }
    public string? Origin { get; set; }
    public string? Destination { get; set; }
    public int? Sequence { get; set; }
    public StripState State { get; set; }
    public bool Cleared { get; set; }

    public string? PositionFrequency { get; set; }

    [ForeignKey( $"{nameof(PositionFrequency)},{nameof(Airport)}")]
    public virtual PositionEntity? Position { get; set; }

    public required string BayName { get; set; }

    [DatabaseGenerated(DatabaseGeneratedOption.Computed)]
    public DateTime UpdatedTime { get; set; }
}
