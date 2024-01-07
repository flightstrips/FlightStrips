using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;
using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

[PrimaryKey(nameof(Callsign), nameof(Session), nameof(Airport))]
public class StripEntity : IAirportAndSessionTenant
{
    [MaxLength(32)]
    public required string Session { get; set; }

    [MaxLength(4)]
    public required string Airport { get; set; }

    [MaxLength(32)]
    public required string Callsign { get; set; }

    [MaxLength(4)]
    public string? Origin { get; set; }
    [MaxLength(4)]
    public string? Destination { get; set; }
    public int? Sequence { get; set; }
    public StripState State { get; set; }
    public bool Cleared { get; set; }

    [MaxLength(7)]
    public string? PositionFrequency { get; set; }

    [MaxLength(32)]
    public required string BayName { get; set; }

    [DatabaseGenerated(DatabaseGeneratedOption.Computed)]
    public DateTime UpdatedTime { get; set; }
}
