
using System.ComponentModel.DataAnnotations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public class Strip
{
    [Required]
    public required StripId Id { get; set; }

    public string Origin { get; set; } = string.Empty;
    public string Destination { get; set; } = string.Empty;

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

    public string? PositionFrequency { get; set; }

    public required string Bay { get; set; }

    public string TOBT { get; set; } = string.Empty;

    public string? TSAT { get; set; }

    public string? TTOT { get; set; }

    public string? CTOT { get; set; }

    public string? AOBT { get; set; }

    public string? ASAT { get; set; }
}
