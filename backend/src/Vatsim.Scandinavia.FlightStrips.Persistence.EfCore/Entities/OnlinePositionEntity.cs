using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;
using Microsoft.EntityFrameworkCore;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

[PrimaryKey(nameof(PositionName), nameof(Session), nameof(Airport))]
public class OnlinePositionEntity : IAirportAndSessionTenant
{
    [MaxLength(32)]
    public required string Session { get; set; }

    [MaxLength(4)]
    public required string Airport { get; set; }

    [MaxLength(32)]
    public required string PositionName { get; set; }

    [MaxLength(7)]
    public required string PositionFrequency { get; set; }

    [Timestamp]
    public uint Version { get; set; }
}
