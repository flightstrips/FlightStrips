using System.ComponentModel.DataAnnotations;
using Microsoft.EntityFrameworkCore;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

[PrimaryKey(nameof(Session), nameof(Airport))]
public class RunwayConfigurationEntity
{
    [MaxLength(4)]
    public required string Airport { get; set; } = string.Empty;
    [MaxLength(32)]
    public required string Session { get; set; } = string.Empty;

    [MaxLength(4)]
    public string Departure { get; set; } = string.Empty;

    [MaxLength(4)]
    public string Arrival { get; set; } = string.Empty;

    [MaxLength(32)]
    public string Position { get; set; } = string.Empty;
}
