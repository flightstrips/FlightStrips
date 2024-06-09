using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips.Events;

public class FullStripEvent
{
    public required StripId Id { get; set; }
    public string Origin { get; set; } = string.Empty;
    public string Destination { get; set; } = string.Empty;

    public string Alternate { get; set; } = string.Empty;

    public string Route { get; set; } = string.Empty;

    public string Remarks { get; set; } = string.Empty;

    public string AssignedSquawk { get; set; } = string.Empty;

    public string Sid { get; set; } = string.Empty;

    public int ClearedAltitude { get; set; }

    public int? Heading { get; set; }

    public string AircraftType { get; set; } = string.Empty;

    public string Runway { get; set; } = string.Empty;

    public int FinalAltitude { get; set; }

    public AircraftCapabilities Capabilities { get; set; } = AircraftCapabilities.Unknown;

    public CommunicationType CommunicationType { get; set; } = CommunicationType.Unassigned;

    public WeightCategory AircraftCategory { get; set; } = WeightCategory.Unknown;

    public StripState State { get; set; } = StripState.None;

    public bool Cleared { get; set; }

    public string TOBT { get; set; } = string.Empty;

    public Position Position { get; set; } = new();

}
