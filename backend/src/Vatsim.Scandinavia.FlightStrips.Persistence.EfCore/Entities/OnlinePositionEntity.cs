using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;
using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Sectors;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

[PrimaryKey(nameof(PositionName), nameof(Session), nameof(Airport))]
public class OnlinePositionEntity
{
    [MaxLength(32)]
    public required string Session { get; set; }

    [MaxLength(4)]
    public required string Airport { get; set; }

    [MaxLength(32)]
    public required string PositionName { get; set; }

    [MaxLength(7)]
    public required string PositionFrequency { get; set; }

    public bool FromPlugin { get; set; }

    public bool ConnectedWithUi { get; set; }

    [MaxLength(3)]
    public string? ArrivalRunway { get; set; }

    [MaxLength(3)]
    public string? DepartureRunway { get; set; }

    public Sector Sector { get; set; } = Sector.NONE;

    [Timestamp]
    public uint Version { get; set; }
}
