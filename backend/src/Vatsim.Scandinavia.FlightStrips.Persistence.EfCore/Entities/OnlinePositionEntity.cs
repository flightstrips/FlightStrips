using System.ComponentModel.DataAnnotations.Schema;
using Microsoft.EntityFrameworkCore;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

[PrimaryKey(nameof(PositionName), nameof(Session), nameof(Airport))]
public class OnlinePositionEntity : IAirportAndSessionTenant
{
    public string Session { get; set; } = string.Empty;

    public string Airport { get; set; } = string.Empty;

    public required string PositionName { get; set; }

    public required string PositionFrequency { get; set; }

    [ForeignKey( $"{nameof(PositionFrequency)},{nameof(Airport)}")]
    public PositionEntity Position { get; set; } = null!;

    // TODO send pings to ensure client is still online or check using VATSIM API
    [DatabaseGenerated(DatabaseGeneratedOption.Computed)]
    public DateTime UpdatedTime { get; set; }
}
