using System.ComponentModel.DataAnnotations;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class StripResponseModel
{
    [Required]
    public required string Callsign { get; set; }
    public required string Origin { get; set; }
    public required string Destination { get; set; }
    public required string Alternate { get; set; }
    public required string Route { get; set; }
    public required string Remarks { get; set; }
    public required string AssignedSquawk { get; set; }
    public required string Squawk { get; set; }
    public string? Sid {get; set; }
    public int? ClearedAltitude { get; set; }
    public int FinalAltitude { get; set; }
    public int? Heading { get; set; }

    public required WeightCategory AircraftCategory { get; set; }
    public required string AircraftType { get; set; }
    public required string Runway { get; set; }
    public required string Capabilities { get; set; }
    public required CommunicationType CommunicationType { get; set; }
    public required string Stand { get; set; }

    public required string Tobt { get; set; }

    public required int Height { get; set; }

    public required double Latitude { get; set; }

    public required double Longitude { get; set; }

    public string? Tsat { get; set; }

    public int? Sequence { get; set; }
    public bool Cleared { get; set; }

    public string? Controller { get; set; }

    [Required]
    public required string Bay { get; set; } = string.Empty;
}
