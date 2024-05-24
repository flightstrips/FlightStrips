using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;
using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

[PrimaryKey(nameof(Callsign), nameof(Session), nameof(Airport))]
public class StripEntity
{
    [MaxLength(32)]
    public required string Session { get; set; }

    [MaxLength(4)]
    public required string Airport { get; set; }

    [MaxLength(32)]
    public required string Callsign { get; set; }

    [MaxLength(4)]
    public required string Origin { get; set; }
    [MaxLength(4)]
    public required string Destination { get; set; }

    [MaxLength(4)]
    public string Alternate { get; set; } = string.Empty;

    public string Route { get; set; } = string.Empty;

    public string Remarks { get; set; } = string.Empty;

    public string AssignedSquawk { get; set; } = string.Empty;

    public string Squawk { get; set; } = string.Empty;

    public string Sid { get; set; } = string.Empty;

    public int ClearedAltitude { get; set; }

    public int? Heading { get; set; }

    public string AircraftType { get; set; } = string.Empty;

    public string Runway { get; set; } = string.Empty;

    public int FinalAltitude { get; set; }


    public string Capabilities { get; set; } = string.Empty;

    public CommunicationType CommunicationType { get; set; } = CommunicationType.Unassigned;

    public WeightCategory AircraftCategory { get; set; } = WeightCategory.Unknown;

    public string Stand { get; set; } = string.Empty;

    public int? Sequence { get; set; }
    public StripState State { get; set; }
    public bool Cleared { get; set; }

    [MaxLength(7)]
    public string? PositionFrequency { get; set; }

    [MaxLength(32)]
    public required string BayName { get; set; }

    [Timestamp]
    public uint Version { get; set; }

    public string TOBT { get; set; } = string.Empty;

    public string? TSAT { get; set; }

    public string? TTOT { get; set; }

    public string? CTOT { get; set; }

    public string? AOBT { get; set; }

    public string? ASAT { get; set; }
}
